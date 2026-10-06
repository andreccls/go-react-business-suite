package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/httpapi"
)

func TestBodyErrors(t *testing.T) {
	e := newEnv(t)
	post := func(body string, headers ...string) resp {
		return e.do("POST", "/v1/services", e.staffTk, body, headers...)
	}
	e.wantProblem(post(`{"name":`), 400, "invalid_json")
	e.wantProblem(post(`{"name":"Maria","unknown_field":1}`), 400, "invalid_json")
	e.wantProblem(post(`{"name":123}`), 400, "invalid_json")
	e.wantProblem(post(``), 400, "invalid_json")
	e.wantProblem(post(`{"name":"Maria"} {"again":true}`), 400, "invalid_json")
	e.wantProblem(post(`{"name":"Maria"} garbage`), 400, "invalid_json")
	e.wantProblem(post(`{}`, "Content-Type", "text/plain"), 415, "unsupported_media_type")
	e.wantProblem(post(`{"name":"`+strings.Repeat("x", 2<<20)+`"}`), 413, "body_too_large")
}

func TestEveryBodyEndpointRejectsMalformedJSON(t *testing.T) {
	e := newEnv(t)
	for _, op := range httpapi.Operations() {
		if op.Method != "POST" && op.Method != "PATCH" {
			continue
		}
		path := strings.ReplaceAll(op.Path, "{id}", unknownID)
		token := e.adminTk
		if op.Access == httpapi.Public {
			token = ""
		}
		e.wantProblem(e.do(op.Method, path, token, `{"broken":`), 400, "invalid_json")
	}
}

// brokenCatalog fails List with a non-domain error and panics on Get.
type brokenCatalog struct {
	httpapi.CatalogService
	err error
}

func (b brokenCatalog) List(context.Context, catalog.Filter) ([]catalog.Item, int, error) {
	return nil, 0, b.err
}
func (b brokenCatalog) Get(context.Context, string) (catalog.Item, error) { panic("kaboom") }

func TestUnexpectedErrorsAreHidden(t *testing.T) {
	e := newEnv(t, func(o *options) {
		o.mutate = func(d *httpapi.Deps) {
			d.Catalog = brokenCatalog{err: errors.New("pq: password authentication failed for user secret")}
		}
	})
	r := e.do("GET", "/v1/services", e.staffTk, nil)
	p := e.wantProblem(r, 500, "internal_error")
	if strings.Contains(string(r.body), "password authentication") {
		t.Errorf("internal error leaked to the client: %v", p)
	}
	if !strings.Contains(e.logs.String(), "password authentication failed") {
		t.Error("the real cause must be logged")
	}
}

func TestTimeoutMapsTo503(t *testing.T) {
	e := newEnv(t, func(o *options) {
		o.mutate = func(d *httpapi.Deps) { d.Catalog = brokenCatalog{err: context.DeadlineExceeded} }
	})
	e.wantProblem(e.do("GET", "/v1/services", e.staffTk, nil), 503, "timeout")
}

func TestPanicIsRecovered(t *testing.T) {
	e := newEnv(t, func(o *options) { o.mutate = func(d *httpapi.Deps) { d.Catalog = brokenCatalog{} } })
	e.wantProblem(e.do("GET", "/v1/services/"+unknownID, e.staffTk, nil), 500, "internal_error")
	if !strings.Contains(e.logs.String(), "panic recovered") {
		t.Error("panic must be logged")
	}
	if r := e.do("GET", "/healthz", "", nil); r.status != 200 {
		t.Errorf("healthz after panic: %d", r.status)
	}
}

func TestRequestIDAndAccessLog(t *testing.T) {
	e := newEnv(t)
	generated := e.do("GET", "/healthz", "", nil).header.Get("X-Request-ID")
	if len(generated) != 36 {
		t.Errorf("generated id = %q", generated)
	}
	if got := e.do("GET", "/healthz", "", nil, "X-Request-ID", "client-id.123").header.Get("X-Request-ID"); got != "client-id.123" {
		t.Errorf("client id not kept: %q", got)
	}
	for _, evil := range []string{"has space", "new\nline", strings.Repeat("a", 65), "<script>"} {
		if got := e.do("GET", "/healthz", "", nil, "X-Request-ID", evil).header.Get("X-Request-ID"); got == evil {
			t.Errorf("unsafe id %q was echoed", evil)
		}
	}
	p := e.wantProblem(e.do("GET", "/v1/services", "", nil, "X-Request-ID", "trace-me"), 401, "missing_token")
	if p["request_id"] != "trace-me" {
		t.Errorf("problem request_id = %v", p["request_id"])
	}
	e.do("GET", "/v1/services", e.staffTk, nil)
	logs := e.logs.String()
	if !strings.Contains(logs, `"request_id":"trace-me"`) || !strings.Contains(logs, `"route":"GET /v1/services"`) ||
		!strings.Contains(logs, `"status":200`) || !strings.Contains(logs, `"user_id":"`) {
		t.Errorf("access log incomplete:\n%s", logs)
	}
}

func TestHealthReadinessAndUnknownRoutes(t *testing.T) {
	e := newEnv(t)
	if r := e.do("GET", "/healthz", "", nil); r.status != 200 || r.json(t)["status"] != "ok" {
		t.Errorf("healthz: %d %s", r.status, r.body)
	}
	if r := e.do("GET", "/readyz", "", nil); r.status != 200 || r.json(t)["status"] != "ready" {
		t.Errorf("readyz: %d %s", r.status, r.body)
	}
	down := newEnv(t, func(o *options) { o.ready = func(context.Context) error { return errors.New("db down") } })
	r := down.do("GET", "/readyz", "", nil)
	down.wantProblem(r, 503, "not_ready")
	if strings.Contains(string(r.body), "db down") {
		t.Error("readiness must not leak the cause")
	}
	e.wantProblem(e.do("GET", "/nope", "", nil), 404, "route_not_found")
}

func TestDocsAreServedFromTheBinary(t *testing.T) {
	e := newEnv(t)
	spec := e.do("GET", "/openapi.json", "", nil)
	if spec.status != 200 || spec.header.Get("Content-Type") != "application/json" || spec.json(t)["openapi"] != "3.0.3" {
		t.Errorf("openapi.json: %d %s", spec.status, spec.header.Get("Content-Type"))
	}
	page := e.do("GET", "/docs/", "", nil)
	if page.status != 200 || !strings.Contains(string(page.body), "swagger-ui-bundle.js") {
		t.Errorf("docs page: %d", page.status)
	}
	if csp := page.header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP = %q", csp)
	}
	for _, f := range []string{"swagger-ui-bundle.js", "swagger-ui.css", "init.js", "LICENSE"} {
		if r := e.do("GET", "/docs/"+f, "", nil); r.status != 200 || len(r.body) == 0 {
			t.Errorf("%s: %d", f, r.status)
		}
	}
	if r := e.do("GET", "/docs", "", nil); r.status != http.StatusMovedPermanently {
		t.Errorf("/docs should redirect to /docs/, got %d", r.status)
	}
	if strings.Contains(string(page.body), "http://") || strings.Contains(string(page.body), "https://") {
		t.Error("docs page must not reference external hosts")
	}
	// The UI fetches the spec relatively, so it also works behind the nginx /api proxy.
	if init := e.do("GET", "/docs/init.js", "", nil); !strings.Contains(string(init.body), `"../openapi.json"`) {
		t.Errorf("init.js must load the spec relatively: %s", init.body)
	}
}

func TestCORS(t *testing.T) {
	const dev = "http://localhost:5173"
	preflight := func(e *env, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("OPTIONS", "/v1/appointments", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "PATCH")
		req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec
	}

	t.Run("allowed origin: preflight and simple requests", func(t *testing.T) {
		e := newEnv(t, func(o *options) { o.origins = []string{dev} })
		rec := preflight(e, dev)
		h := rec.Header()
		if rec.Code != http.StatusNoContent || h.Get("Access-Control-Allow-Origin") != dev || h.Get("Vary") != "Origin" ||
			!strings.Contains(h.Get("Access-Control-Allow-Methods"), "PATCH") || !strings.Contains(h.Get("Access-Control-Allow-Headers"), "Authorization") ||
			h.Get("Access-Control-Max-Age") == "" || h.Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("preflight: %d %v", rec.Code, h)
		}
		r := e.do("GET", "/healthz", "", nil, "Origin", dev)
		if r.header.Get("Access-Control-Allow-Origin") != dev || !strings.Contains(r.header.Get("Access-Control-Expose-Headers"), "X-Request-ID") {
			t.Errorf("simple request headers: %v", r.header)
		}
	})
	t.Run("preflight needs no token and spends no rate limit", func(t *testing.T) {
		e := newEnv(t, func(o *options) { o.origins = []string{dev}; o.apiLimit = 1 })
		for i := 0; i < 3; i++ {
			if rec := preflight(e, dev); rec.Code != http.StatusNoContent {
				t.Fatalf("preflight %d: %d", i, rec.Code)
			}
		}
	})
	t.Run("other origins get no CORS headers", func(t *testing.T) {
		e := newEnv(t, func(o *options) { o.origins = []string{dev} })
		if rec := preflight(e, "https://evil.example"); rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Code == http.StatusNoContent {
			t.Errorf("disallowed origin: %d %v", rec.Code, rec.Header())
		}
		if r := e.do("GET", "/healthz", "", nil, "Origin", "https://evil.example"); r.header.Get("Access-Control-Allow-Origin") != "" {
			t.Error("simple request from a disallowed origin got CORS headers")
		}
	})
	t.Run("off by default", func(t *testing.T) {
		e := newEnv(t)
		if r := e.do("GET", "/healthz", "", nil, "Origin", dev); r.header.Get("Access-Control-Allow-Origin") != "" {
			t.Error("CORS must be off when no origin is configured")
		}
	})
	t.Run("wildcard", func(t *testing.T) {
		e := newEnv(t, func(o *options) { o.origins = []string{"*"} })
		r := e.do("GET", "/healthz", "", nil, "Origin", "https://anything.example")
		if r.header.Get("Access-Control-Allow-Origin") != "*" || r.header.Get("Vary") != "" {
			t.Errorf("wildcard: %v", r.header)
		}
	})
	t.Run("a non-preflight OPTIONS passes through", func(t *testing.T) {
		e := newEnv(t, func(o *options) { o.origins = []string{dev} })
		req := httptest.NewRequest("OPTIONS", "/v1/services", nil)
		req.Header.Set("Origin", dev)
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		if rec.Code == http.StatusNoContent {
			t.Error("OPTIONS without Access-Control-Request-Method is not a preflight")
		}
	})
}
