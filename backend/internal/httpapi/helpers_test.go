package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/andreccls/go-react-business-suite/backend/internal/auth"
	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
	"github.com/andreccls/go-react-business-suite/backend/internal/dashboard"
	"github.com/andreccls/go-react-business-suite/backend/internal/httpapi"
	"github.com/andreccls/go-react-business-suite/backend/internal/memstore"
	"github.com/andreccls/go-react-business-suite/backend/internal/ratelimit"
)

const (
	adminEmail = "admin@example.com"
	adminPass  = "admin-pass-1234"
	staffEmail = "staff@example.com"
	staffPass  = "staff-pass-1234"
)

// ---------------------------------------------------------------------------
// The OpenAPI document as an oracle: every request the tests make is checked against
// it — the status must be declared for that operation, the content type too, and the
// JSON body must match the declared schema (types, required, no undeclared fields).
// ---------------------------------------------------------------------------

type specIndex struct {
	root map[string]any
	ops  map[string]map[string]any // "METHOD /path/{id}" -> responses
	re   map[string]*regexp.Regexp
}

func loadSpec(t testing.TB) specIndex {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(httpapi.OpenAPI(), &root); err != nil {
		t.Fatalf("openapi.json is not valid JSON: %v", err)
	}
	idx := specIndex{root: root, ops: map[string]map[string]any{}, re: map[string]*regexp.Regexp{}}
	for path, item := range root["paths"].(map[string]any) {
		for method, raw := range item.(map[string]any) {
			if method == "parameters" {
				continue
			}
			key := strings.ToUpper(method) + " " + path
			idx.ops[key] = raw.(map[string]any)["responses"].(map[string]any)
			idx.re[key] = regexp.MustCompile("^" + regexp.MustCompile(`\\\{[^/]+\\\}`).ReplaceAllString(regexp.QuoteMeta(path), `[^/]+`) + "$")
		}
	}
	return idx
}

// resolve follows a local $ref.
func (s specIndex) resolve(v any) any {
	for {
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		ref, ok := m["$ref"].(string)
		if !ok {
			return v
		}
		cur := any(s.root)
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			cur = cur.(map[string]any)[part]
		}
		v = cur
	}
}

// check validates a response against the spec.
func (s specIndex) check(t testing.TB, method, path string, status int, contentType string, body []byte) {
	t.Helper()
	path, _, _ = strings.Cut(path, "?")
	for key, re := range s.re {
		if !strings.HasPrefix(key, method+" ") || !re.MatchString(path) {
			continue
		}
		raw, ok := s.ops[key][strconv.Itoa(status)]
		if !ok {
			t.Errorf("%s %s answered %d, which openapi.json does not declare for %s", method, path, status, key)
			return
		}
		resp := s.resolve(raw).(map[string]any)
		content, _ := resp["content"].(map[string]any)
		if len(content) == 0 {
			if len(body) != 0 {
				t.Errorf("%s %s %d: the spec declares no body but got %q", method, path, status, body)
			}
			return
		}
		mt, _, _ := mime.ParseMediaType(contentType)
		media, ok := content[mt].(map[string]any)
		if !ok {
			t.Errorf("%s %s %d: content type %q is not declared in the spec", method, path, status, contentType)
			return
		}
		var doc any
		if err := json.Unmarshal(body, &doc); err != nil {
			t.Errorf("%s %s %d: body is not JSON: %v", method, path, status, err)
			return
		}
		for _, problem := range s.validate(media["schema"], doc, "$") {
			t.Errorf("%s %s %d: body does not match the spec: %s\n%s", method, path, status, problem, body)
		}
		return
	}
}

var (
	dateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// validate is a small JSON-Schema checker for the subset the document uses.
func (s specIndex) validate(schema any, v any, at string) (problems []string) {
	schema = s.resolve(schema)
	sc, ok := schema.(map[string]any)
	if !ok {
		return []string{at + ": no schema"}
	}
	if all, ok := sc["allOf"].([]any); ok {
		for _, sub := range all {
			problems = append(problems, s.validate(sub, v, at)...)
		}
	}
	if enum, ok := sc["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			found = found || e == v
		}
		if !found {
			problems = append(problems, fmt.Sprintf("%s: %v is not one of %v", at, v, enum))
		}
	}
	switch sc["type"] {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return append(problems, at+": expected an object")
		}
		props, _ := sc["properties"].(map[string]any)
		if req, ok := sc["required"].([]any); ok {
			for _, r := range req {
				if _, present := m[r.(string)]; !present {
					problems = append(problems, fmt.Sprintf("%s: missing required field %q", at, r))
				}
			}
		}
		for k, val := range m {
			sub, declared := props[k]
			if !declared {
				problems = append(problems, fmt.Sprintf("%s: undeclared field %q", at, k))
				continue
			}
			problems = append(problems, s.validate(sub, val, at+"."+k)...)
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			return append(problems, at+": expected an array (null is not allowed)")
		}
		for i, el := range arr {
			problems = append(problems, s.validate(sc["items"], el, fmt.Sprintf("%s[%d]", at, i))...)
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			return append(problems, at+": expected a string")
		}
		switch sc["format"] {
		case "date-time":
			if _, err := time.Parse(time.RFC3339Nano, str); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %q is not a date-time", at, str))
			}
		case "date":
			if !dateRE.MatchString(str) {
				problems = append(problems, fmt.Sprintf("%s: %q is not a date", at, str))
			}
		case "uuid":
			if _, err := uuid.Parse(str); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %q is not a uuid", at, str))
			}
		}
	case "integer":
		f, ok := v.(float64)
		if !ok || f != float64(int64(f)) {
			problems = append(problems, fmt.Sprintf("%s: %v is not an integer", at, v))
		}
	case "number":
		if _, ok := v.(float64); !ok {
			problems = append(problems, at+": expected a number")
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			problems = append(problems, at+": expected a boolean")
		}
	}
	return problems
}

// ---------------------------------------------------------------------------
// The test environment: the real handler on in-memory stores and a fake clock.
// ---------------------------------------------------------------------------

type env struct {
	t        *testing.T
	h        http.Handler
	spec     specIndex
	logs     *bytes.Buffer
	now      time.Time // the clock of the booking and dashboard services
	adminTk  string
	staffTk  string
	catalog  *catalog.Service
	custs    *customer.Service
	bookings *booking.Service
}

type options struct {
	authLimit   int
	apiLimit    int
	ready       func(context.Context) error
	timeout     time.Duration
	skipSeeding bool
	origins     []string
	mutate      func(*httpapi.Deps)
}

// t0 is "now" for the fake clock: Monday 2026-03-02 12:00 UTC (09:00 in São Paulo).
var t0 = time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)

func newEnv(t *testing.T, mods ...func(*options)) *env {
	t.Helper()
	o := options{authLimit: 1000, apiLimit: 1000, ready: func(context.Context) error { return nil }, timeout: 5 * time.Second}
	for _, m := range mods {
		m(&o)
	}
	sp, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, spec: loadSpec(t), logs: &bytes.Buffer{}, now: t0}
	clock := func() time.Time { return e.now }

	db := memstore.New()
	authSvc := auth.NewService(memstore.NewUsers(), auth.Config{
		Secret: []byte("integration-test-secret-0123456789abcdef"), Issuer: "test",
		AccessTTL: 15 * time.Minute, RefreshTTL: time.Hour, BcryptCost: bcrypt.MinCost,
	}, nil)
	e.catalog = catalog.NewService(db.Catalog(), clock)
	e.custs = customer.NewService(db.Customers(), clock)
	e.bookings = booking.NewService(db.Appointments(), e.catalog, e.custs, sp, clock)
	deps := httpapi.Deps{
		Catalog: e.catalog, Customers: e.custs, Bookings: e.bookings,
		Dashboard:      dashboard.NewService(db.Dashboard(), db.Appointments(), sp, clock),
		Auth:           authSvc,
		Ready:          o.ready,
		Logger:         slog.New(slog.NewJSONHandler(e.logs, nil)),
		AuthLimiter:    ratelimit.New(o.authLimit, nil),
		APILimiter:     ratelimit.New(o.apiLimit, nil),
		RequestTimeout: o.timeout,
		CORSOrigins:    o.origins,
	}
	if o.mutate != nil {
		o.mutate(&deps)
	}
	e.h = httpapi.New(deps)

	if !o.skipSeeding {
		ctx := context.Background()
		if err := authSvc.EnsureAdmin(ctx, adminEmail, adminPass); err != nil {
			t.Fatal(err)
		}
		if _, err := authSvc.CreateUser(ctx, staffEmail, staffPass, auth.RoleStaff); err != nil {
			t.Fatal(err)
		}
		a, _ := authSvc.Login(ctx, adminEmail, adminPass)
		s, _ := authSvc.Login(ctx, staffEmail, staffPass)
		e.adminTk, e.staffTk = a.AccessToken, s.AccessToken
	}
	return e
}

type resp struct {
	status int
	header http.Header
	body   []byte
}

func (r resp) json(t testing.TB) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("body is not a JSON object: %q", r.body)
	}
	return m
}

// do sends a request; body may be a string (raw), nil, or any JSON-encodable value.
func (e *env) do(method, path, token string, body any, headers ...string) resp {
	e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	e.spec.check(e.t, method, path, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes())
	return resp{status: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
}

// wantProblem asserts an RFC 9457 response with the given status and code.
func (e *env) wantProblem(r resp, status int, code string) map[string]any {
	e.t.Helper()
	if r.status != status {
		e.t.Fatalf("status = %d, want %d (body %s)", r.status, status, r.body)
	}
	if ct := r.header.Get("Content-Type"); ct != "application/problem+json" {
		e.t.Errorf("Content-Type = %q", ct)
	}
	p := r.json(e.t)
	if p["code"] != code || p["type"] != "about:blank" || int(p["status"].(float64)) != status || p["title"] != http.StatusText(status) {
		e.t.Errorf("problem = %v, want code %q", p, code)
	}
	return p
}

// created POSTs and returns the body of the 201.
func (e *env) created(path, token string, body any) map[string]any {
	e.t.Helper()
	r := e.do("POST", path, token, body)
	if r.status != http.StatusCreated {
		e.t.Fatalf("POST %s = %d %s", path, r.status, r.body)
	}
	return r.json(e.t)
}

func (e *env) newService(name string, minutes, cents int) map[string]any {
	return e.created("/v1/services", e.staffTk, map[string]any{"name": name, "duration_min": minutes, "price_cents": cents})
}

func (e *env) newCustomer(n int) map[string]any {
	return e.created("/v1/customers", e.staffTk, map[string]any{"name": "Customer " + strconv.Itoa(n), "email": "c" + strconv.Itoa(n) + "@example.com", "phone": "(31) 99999-0000"})
}

// bookAt books an appointment `in` from the fake now.
func (e *env) bookAt(cust, svc map[string]any, in time.Duration) resp {
	return e.do("POST", "/v1/appointments", e.staffTk, map[string]any{
		"customer_id": cust["id"], "service_id": svc["id"], "starts_at": e.now.Add(in).Format(time.RFC3339),
	})
}
