package httpapi_test

import (
	"testing"
	"time"
)

// A tiny, hand-checkable scenario driven only through the API. The clock is moved
// forward so appointments can be completed.
func TestDashboardEndpoints(t *testing.T) {
	e := newEnv(t)
	cut, color := e.newService("Cut", 30, 5000), e.newService("Color", 60, 20000)
	ana, bia := e.newCustomer(1), e.newCustomer(2)

	book := func(cus, svc map[string]any, at string) string {
		t.Helper()
		r := e.do("POST", "/v1/appointments", e.staffTk, map[string]any{"customer_id": cus["id"], "service_id": svc["id"], "starts_at": at})
		if r.status != 201 {
			t.Fatalf("book %s: %d %s", at, r.status, r.body)
		}
		return r.json(t)["id"].(string)
	}
	set := func(id, status string) {
		t.Helper()
		if r := e.do("PATCH", "/v1/appointments/"+id+"/status", e.staffTk, map[string]any{"status": status}); r.status != 200 {
			t.Fatalf("%s: %d %s", status, r.status, r.body)
		}
	}
	// Local days (São Paulo): 03-03 and 03-04.
	a1 := book(ana, cut, "2026-03-03T13:00:00Z")   // completed  5000
	a2 := book(bia, color, "2026-03-03T15:00:00Z") // completed 20000
	a3 := book(ana, cut, "2026-03-04T13:00:00Z")   // cancelled
	a4 := book(bia, cut, "2026-03-04T15:00:00Z")   // no_show
	book(ana, cut, "2026-03-20T13:00:00Z")         // scheduled, far in the future (upcoming)

	e.now = time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
	set(a1, "completed")
	set(a2, "completed")
	set(a3, "cancelled")
	set(a4, "no_show")
	e.now = t0 // customers were created "now" (03-02); keep the clock there for the period checks below

	sum := e.do("GET", "/v1/dashboard/summary?from=2026-03-01&to=2026-03-31", e.staffTk, nil).json(t)
	by := sum["by_status"].(map[string]any)
	if sum["from"] != "2026-03-01" || sum["to"] != "2026-03-31" || sum["timezone"] != "America/Sao_Paulo" ||
		sum["appointments_total"] != 5.0 || sum["revenue_cents"] != 25000.0 || sum["average_ticket_cents"] != 12500.0 ||
		sum["cancellation_rate"] != 0.2 || sum["no_show_rate"] != 0.2 || sum["new_customers"] != 2.0 ||
		by["completed"] != 2.0 || by["cancelled"] != 1.0 || by["no_show"] != 1.0 || by["scheduled"] != 1.0 {
		t.Errorf("summary = %v", sum)
	}

	daily := e.do("GET", "/v1/dashboard/daily?from=2026-03-02&to=2026-03-05", e.staffTk, nil).json(t)
	days := daily["data"].([]any)
	if len(days) != 4 {
		t.Fatalf("daily = %v", daily)
	}
	d := func(i int) map[string]any { return days[i].(map[string]any) }
	if d(0)["date"] != "2026-03-02" || d(0)["appointments"] != 0.0 ||
		d(1)["date"] != "2026-03-03" || d(1)["appointments"] != 2.0 || d(1)["completed"] != 2.0 || d(1)["revenue_cents"] != 25000.0 ||
		d(2)["date"] != "2026-03-04" || d(2)["appointments"] != 2.0 || d(2)["completed"] != 0.0 ||
		d(3)["date"] != "2026-03-05" || d(3)["appointments"] != 0.0 {
		t.Errorf("daily = %v", days)
	}

	top := e.do("GET", "/v1/dashboard/top-services?from=2026-03-01&to=2026-03-31&limit=1", e.staffTk, nil).json(t)
	rows := top["data"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["name"] != "Color" || rows[0].(map[string]any)["revenue_cents"] != 20000.0 {
		t.Errorf("top = %v", top)
	}
	all := e.do("GET", "/v1/dashboard/top-services?from=2026-03-01&to=2026-03-31", e.staffTk, nil).json(t)["data"].([]any)
	if len(all) != 2 || all[1].(map[string]any)["name"] != "Cut" || all[1].(map[string]any)["appointments"] != 4.0 || all[1].(map[string]any)["completed"] != 1.0 {
		t.Errorf("top (all) = %v", all)
	}

	up := e.do("GET", "/v1/dashboard/upcoming", e.staffTk, nil).json(t)["data"].([]any)
	if len(up) != 1 || up[0].(map[string]any)["status"] != "scheduled" || up[0].(map[string]any)["starts_at"] != "2026-03-20T13:00:00Z" {
		t.Errorf("upcoming = %v", up)
	}

	// Defaults: the last 30 days ending today (03-02 local).
	def := e.do("GET", "/v1/dashboard/summary", e.staffTk, nil).json(t)
	if def["to"] != "2026-03-02" || def["from"] != "2026-02-01" {
		t.Errorf("defaults = %v..%v", def["from"], def["to"])
	}
}

func TestDashboardParameterValidation(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{
		"/v1/dashboard/summary?from=2026-03-10&to=2026-03-01",
		"/v1/dashboard/summary?from=x",
		"/v1/dashboard/summary?from=2024-01-01&to=2026-01-01",
		"/v1/dashboard/daily?to=31-03-2026",
		"/v1/dashboard/top-services?limit=0x",
		"/v1/dashboard/top-services?limit=21",
		"/v1/dashboard/top-services?from=bad",
		"/v1/dashboard/upcoming?limit=abc",
		"/v1/dashboard/upcoming?limit=-1",
	} {
		e.wantProblem(e.do("GET", path, e.staffTk, nil), 422, "validation_failed")
	}
	// Empty data is well-formed: zeros, [] — never null or NaN.
	empty := e.do("GET", "/v1/dashboard/summary", e.staffTk, nil).json(t)
	if empty["appointments_total"] != 0.0 || empty["average_ticket_cents"] != 0.0 || empty["cancellation_rate"] != 0.0 {
		t.Errorf("empty summary = %v", empty)
	}
	if d := e.do("GET", "/v1/dashboard/upcoming", e.staffTk, nil).json(t)["data"]; d == nil {
		t.Error("upcoming data must be [] not null")
	}
	if d := e.do("GET", "/v1/dashboard/top-services", e.staffTk, nil).json(t)["data"]; d == nil {
		t.Error("top services data must be [] not null")
	}
	e.wantProblem(e.do("GET", "/v1/dashboard/summary", "", nil), 401, "missing_token")
}
