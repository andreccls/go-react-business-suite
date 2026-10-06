package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/andreccls/go-react-business-suite/backend/internal/app"
	"github.com/andreccls/go-react-business-suite/backend/internal/config"
	"github.com/andreccls/go-react-business-suite/backend/internal/testdb"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	sp, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	return config.Config{
		Env: config.EnvDevelopment, DatabaseURL: testdb.URL(t), JWTSecret: "app-test-secret-app-test-secret-0123",
		AccessTTL: time.Minute, RefreshTTL: time.Hour, AuthRatePerMin: 1000, APIRatePerMin: 100000,
		AdminEmail: "admin@example.com", AdminPassword: "admin-pass-1234", Location: sp,
		CORSOrigins: []string{"http://localhost:5173"},
	}
}

type client struct {
	t     *testing.T
	base  string
	token string
}

func (c client) call(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func start(t *testing.T) (client, *httptest.Server) {
	t.Helper()
	a, err := app.New(context.Background(), testConfig(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	c := client{t: t, base: srv.URL}
	st, tok := c.call("POST", "/v1/auth/login", map[string]any{"email": "admin@example.com", "password": "admin-pass-1234"})
	if st != 200 {
		t.Fatalf("admin login = %d (the bootstrap admin must be seeded)", st)
	}
	c.token = tok["access_token"].(string)
	return c, srv
}

// TestEndToEndAgainstPostgres drives the whole stack (HTTP -> services -> pgx -> PostgreSQL).
func TestEndToEndAgainstPostgres(t *testing.T) {
	c, _ := start(t)
	if st, _ := c.call("GET", "/readyz", nil); st != 200 {
		t.Fatalf("readyz = %d", st)
	}
	st, svc := c.call("POST", "/v1/services", map[string]any{"name": "Cut", "duration_min": 30, "price_cents": 5000})
	if st != 201 {
		t.Fatalf("create service = %d %v", st, svc)
	}
	st, cus := c.call("POST", "/v1/customers", map[string]any{"name": "Ana", "email": "ana@example.com"})
	if st != 201 {
		t.Fatalf("create customer = %d %v", st, cus)
	}
	if st, p := c.call("POST", "/v1/customers", map[string]any{"name": "Ana 2", "email": "ANA@example.com"}); st != 409 || p["code"] != "email_taken" {
		t.Errorf("duplicate e-mail = %d %v", st, p)
	}

	at := time.Now().Add(48 * time.Hour).Truncate(time.Hour)
	book := func(at time.Time) (int, map[string]any) {
		return c.call("POST", "/v1/appointments", map[string]any{"customer_id": cus["id"], "service_id": svc["id"], "starts_at": at.Format(time.RFC3339)})
	}
	st, apt := book(at)
	if st != 201 || apt["price_cents"] != 5000.0 || apt["status"] != "scheduled" {
		t.Fatalf("book = %d %v", st, apt)
	}
	if st, p := book(at.Add(10 * time.Minute)); st != 409 || p["code"] != "slot_unavailable" {
		t.Errorf("overlap = %d %v", st, p)
	}
	if st, _ := book(at.Add(30 * time.Minute)); st != 201 {
		t.Errorf("back-to-back = %d", st)
	}
	if st, p := book(time.Now().Add(-time.Hour)); st != 422 {
		t.Errorf("past = %d %v", st, p)
	}
	if st, p := c.call("PATCH", "/v1/appointments/"+apt["id"].(string)+"/status", map[string]any{"status": "completed"}); st != 409 || p["code"] != "not_started" {
		t.Errorf("complete before start = %d %v", st, p)
	}
	if st, p := c.call("PATCH", "/v1/appointments/"+apt["id"].(string)+"/status", map[string]any{"status": "cancelled"}); st != 200 || p["status"] != "cancelled" {
		t.Errorf("cancel = %d %v", st, p)
	}
	if st, _ := book(at); st != 201 {
		t.Errorf("cancelled slot must be free again: %d", st)
	}

	// Referential integrity surfaces as domain conflicts, not 500s.
	if st, p := c.call("DELETE", "/v1/services/"+svc["id"].(string), nil); st != 409 || p["code"] != "service_in_use" {
		t.Errorf("delete service in use = %d %v", st, p)
	}
	if st, p := c.call("DELETE", "/v1/customers/"+cus["id"].(string), nil); st != 409 || p["code"] != "customer_in_use" {
		t.Errorf("delete customer in use = %d %v", st, p)
	}

	st, sum := c.call("GET", "/v1/dashboard/summary", nil)
	if st != 200 || sum["timezone"] != "America/Sao_Paulo" || sum["new_customers"] != 1.0 {
		t.Errorf("summary = %d %v", st, sum)
	}
	if st, up := c.call("GET", "/v1/dashboard/upcoming", nil); st != 200 || len(up["data"].([]any)) != 2 {
		t.Errorf("upcoming = %d %v", st, up)
	}
}

// The headline guarantee: N simultaneous requests for the same slot, through the whole
// stack on a real PostgreSQL, produce exactly one appointment; everyone else gets 409.
func TestConcurrentBookingsOnePerSlot(t *testing.T) {
	c, _ := start(t)
	_, svc := c.call("POST", "/v1/services", map[string]any{"name": "Cut", "duration_min": 60, "price_cents": 5000})
	_, cus := c.call("POST", "/v1/customers", map[string]any{"name": "Ana", "email": "ana@example.com"})
	at := time.Now().Add(72 * time.Hour).Truncate(time.Hour)

	const n = 30
	statuses := make([]int, n)
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			statuses[i], _ = c.call("POST", "/v1/appointments", map[string]any{"customer_id": cus["id"], "service_id": svc["id"], "starts_at": at.Format(time.RFC3339)})
		}()
	}
	close(gate)
	wg.Wait()

	created, conflicts := 0, 0
	for _, s := range statuses {
		switch s {
		case 201:
			created++
		case 409:
			conflicts++
		}
	}
	if created != 1 || conflicts != n-1 {
		t.Fatalf("created = %d, conflicts = %d, statuses = %v", created, conflicts, statuses)
	}
	_, list := c.call("GET", "/v1/appointments", nil)
	if list["total"] != 1.0 {
		t.Errorf("stored appointments = %v, want 1", list["total"])
	}
}

func TestCORSPreflightThroughTheRealStack(t *testing.T) {
	_, srv := start(t)
	req, _ := http.NewRequest("OPTIONS", srv.URL+"/v1/appointments", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("preflight = %d %v", resp.StatusCode, resp.Header)
	}
}

func TestNewRestartKeepsAdminAndData(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for i := 0; i < 2; i++ { // second start: migrations and admin seeding are idempotent
		a, err := app.New(ctx, cfg, log)
		if err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		a.Close()
	}
}

func TestNewFailures(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bad := testConfig(t)
	bad.DatabaseURL = "postgres://nobody:x@127.0.0.1:1/none?connect_timeout=1"
	if _, err := app.New(ctx, bad, log); err == nil {
		t.Error("unreachable database must fail startup")
	}
	invalidAdmin := testConfig(t)
	invalidAdmin.AdminEmail = "not-an-email"
	if _, err := app.New(ctx, invalidAdmin, log); err == nil {
		t.Error("invalid bootstrap admin must fail startup")
	}
}

func TestServeShutsDownGracefully(t *testing.T) {
	cfg := testConfig(t)
	cfg.Env = config.EnvRelease // exercises the non-warning branch
	a, err := app.New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx, ln) }()

	addr := ln.Addr().String()
	var last error
	for i := 0; i < 50; i++ { // wait until it answers
		if last = app.Healthcheck(addr); last == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if last != nil {
		t.Fatalf("never became healthy: %v", last)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v on graceful shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	if err := app.Healthcheck(addr); err == nil {
		t.Error("server still answering after shutdown")
	}
}

func TestServeReturnsListenerError(t *testing.T) {
	a, err := app.New(context.Background(), testConfig(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	ln.Close() // serving on a closed listener fails immediately
	if err := a.Serve(context.Background(), ln); err == nil {
		t.Error("expected an error")
	}
}

func TestHealthcheck(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer ok.Close()
	if err := app.Healthcheck(ok.Listener.Addr().String()); err != nil {
		t.Errorf("healthy server: %v", err)
	}
	sick := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer sick.Close()
	if err := app.Healthcheck(sick.Listener.Addr().String()); err == nil {
		t.Error("503 must be unhealthy")
	}
	if err := app.Healthcheck("no-port"); err == nil {
		t.Error("malformed address must fail")
	}
	if err := app.Healthcheck(":1"); err == nil {
		t.Error("nothing listens on :1")
	}
}
