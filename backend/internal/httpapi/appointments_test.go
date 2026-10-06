package httpapi_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBookingHappyPathAndSnapshot(t *testing.T) {
	e := newEnv(t)
	svc, cus := e.newService("Deep tissue massage", 60, 12000), e.newCustomer(1)
	r := e.do("POST", "/v1/appointments", e.staffTk, map[string]any{
		"customer_id": cus["id"], "service_id": svc["id"], "starts_at": "2026-03-04T14:00:00-03:00", "notes": " window seat ",
	})
	if r.status != http.StatusCreated {
		t.Fatalf("book: %d %s", r.status, r.body)
	}
	a := r.json(t)
	id := a["id"].(string)
	if r.header.Get("Location") != "/v1/appointments/"+id || a["status"] != "scheduled" || a["starts_at"] != "2026-03-04T17:00:00Z" ||
		a["ends_at"] != "2026-03-04T18:00:00Z" || a["price_cents"] != 12000.0 || a["duration_min"] != 60.0 ||
		a["service_name"] != "Deep tissue massage" || a["customer_name"] != "Customer 1" || a["notes"] != "window seat" {
		t.Errorf("appointment: %v", a)
	}

	// Editing the service afterwards does not touch the appointment.
	e.do("PATCH", "/v1/services/"+svc["id"].(string), e.staffTk, map[string]any{"name": "Renamed", "price_cents": 99900, "duration_min": 120})
	got := e.do("GET", "/v1/appointments/"+id, e.staffTk, nil).json(t)
	if got["service_name"] != "Deep tissue massage" || got["price_cents"] != 12000.0 || got["duration_min"] != 60.0 || got["ends_at"] != "2026-03-04T18:00:00Z" {
		t.Errorf("snapshot changed: %v", got)
	}
}

func TestBookingRules(t *testing.T) {
	e := newEnv(t)
	svc, cus := e.newService("Cut", 30, 5000), e.newCustomer(1)
	off := e.created("/v1/services", e.staffTk, map[string]any{"name": "Retired", "duration_min": 30, "price_cents": 1, "active": false})

	p := e.wantProblem(e.bookAt(cus, svc, -time.Hour), 422, "validation_failed")
	if f := p["errors"].([]any)[0].(map[string]any); f["field"] != "starts_at" || f["message"] != "must be in the future" {
		t.Errorf("past booking: %v", f)
	}
	e.wantProblem(e.bookAt(cus, off, time.Hour), 422, "service_inactive")
	e.wantProblem(e.do("POST", "/v1/appointments", e.staffTk, map[string]any{"customer_id": unknownID, "service_id": svc["id"], "starts_at": "2026-03-04T14:00:00Z"}), 422, "validation_failed")
	e.wantProblem(e.do("POST", "/v1/appointments", e.staffTk, map[string]any{"customer_id": cus["id"], "service_id": unknownID, "starts_at": "2026-03-04T14:00:00Z"}), 422, "validation_failed")
	e.wantProblem(e.do("POST", "/v1/appointments", e.staffTk, map[string]any{"customer_id": cus["id"], "service_id": svc["id"], "starts_at": "tomorrow"}), 400, "invalid_json")
	e.wantProblem(e.do("POST", "/v1/appointments", e.staffTk, map[string]any{"customer_id": "x", "service_id": "y"}), 422, "validation_failed")

	// Overlap -> 409; back-to-back and a cancelled slot are fine.
	first := e.bookAt(cus, svc, 2*time.Hour)
	if first.status != 201 {
		t.Fatal(string(first.body))
	}
	e.wantProblem(e.bookAt(cus, svc, 2*time.Hour), 409, "slot_unavailable")
	e.wantProblem(e.bookAt(cus, svc, 2*time.Hour+15*time.Minute), 409, "slot_unavailable")
	if r := e.bookAt(cus, svc, 2*time.Hour+30*time.Minute); r.status != 201 {
		t.Errorf("back-to-back: %d %s", r.status, r.body)
	}
	e.do("PATCH", "/v1/appointments/"+first.json(t)["id"].(string)+"/status", e.staffTk, map[string]any{"status": "cancelled"})
	if r := e.bookAt(cus, svc, 2*time.Hour); r.status != 201 {
		t.Errorf("cancelled slot must be free: %d %s", r.status, r.body)
	}
}

func TestStatusTransitions(t *testing.T) {
	e := newEnv(t)
	svc, cus := e.newService("Cut", 30, 5000), e.newCustomer(1)
	setStatus := func(id, status string) resp {
		return e.do("PATCH", "/v1/appointments/"+id+"/status", e.staffTk, map[string]any{"status": status})
	}

	a := e.bookAt(cus, svc, 2*time.Hour).json(t)
	id := a["id"].(string)
	// Not before it starts.
	e.wantProblem(setStatus(id, "completed"), 409, "not_started")
	e.wantProblem(setStatus(id, "no_show"), 409, "not_started")
	e.wantProblem(setStatus(id, "scheduled"), 409, "invalid_transition")
	e.wantProblem(setStatus(id, "paid"), 422, "validation_failed")
	e.wantProblem(e.do("PATCH", "/v1/appointments/"+id+"/status", e.staffTk, map[string]any{}), 422, "validation_failed")

	e.now = e.now.Add(3 * time.Hour) // the appointment has started
	done := setStatus(id, "completed")
	if done.status != 200 || done.json(t)["status"] != "completed" {
		t.Fatalf("complete: %d %s", done.status, done.body)
	}
	for _, to := range []string{"cancelled", "no_show", "completed", "scheduled"} {
		e.wantProblem(setStatus(id, to), 409, "invalid_transition")
	}
	e.wantProblem(setStatus(unknownID, "cancelled"), 404, "appointment_not_found")
	e.wantProblem(setStatus("nope", "cancelled"), 404, "appointment_not_found")

	e.now = t0
	c := e.bookAt(cus, svc, 5*time.Hour).json(t)
	if r := setStatus(c["id"].(string), "cancelled"); r.status != 200 || r.json(t)["status"] != "cancelled" {
		t.Errorf("cancel before start: %d %s", r.status, r.body)
	}
}

func TestAppointmentListing(t *testing.T) {
	e := newEnv(t)
	svc := e.newService("Cut", 30, 5000)
	ana, bia := e.newCustomer(1), e.newCustomer(2)
	// Monday 03-02 noon UTC is "now". 03-03 10:00 BRT = 13:00Z; 03-05 22:30 BRT = 03-06 01:30Z.
	book := func(cus map[string]any, at string) string {
		r := e.do("POST", "/v1/appointments", e.staffTk, map[string]any{"customer_id": cus["id"], "service_id": svc["id"], "starts_at": at})
		if r.status != 201 {
			t.Fatalf("book %s: %d %s", at, r.status, r.body)
		}
		return r.json(t)["id"].(string)
	}
	a1 := book(ana, "2026-03-03T13:00:00Z")
	a2 := book(bia, "2026-03-06T01:30:00Z")
	a3 := book(ana, "2026-03-10T13:00:00Z")
	e.do("PATCH", "/v1/appointments/"+a3+"/status", e.staffTk, map[string]any{"status": "cancelled"})

	ids := func(q string) string {
		t.Helper()
		r := e.do("GET", "/v1/appointments"+q, e.staffTk, nil)
		if r.status != 200 {
			t.Fatalf("list %s: %d %s", q, r.status, r.body)
		}
		var out []string
		for _, it := range r.json(t)["data"].([]any) {
			switch it.(map[string]any)["id"] {
			case a1:
				out = append(out, "a1")
			case a2:
				out = append(out, "a2")
			case a3:
				out = append(out, "a3")
			}
		}
		return strings.Join(out, ",")
	}
	for q, want := range map[string]string{
		"":                                      "a1,a2,a3",
		"?status=cancelled":                     "a3",
		"?customer_id=" + bia["id"].(string):    "a2",
		"?from=2026-03-05&to=2026-03-05":        "a2", // 01:30Z on the 6th is still the 5th in São Paulo
		"?from=2026-03-06":                      "a3",
		"?to=2026-03-03":                        "a1",
		"?from=2026-03-01&to=2026-03-31&page=1": "a1,a2,a3",
		"?from=2027-01-01&to=2027-01-02":        "",
	} {
		if got := ids(q); got != want {
			t.Errorf("list%s = %q, want %q", q, got, want)
		}
	}
	r := e.do("GET", "/v1/appointments?page=2&page_size=2", e.staffTk, nil).json(t)
	if r["total"] != 3.0 || len(r["data"].([]any)) != 1 || r["data"].([]any)[0].(map[string]any)["customer_name"] != "Customer 1" {
		t.Errorf("page 2: %v", r)
	}
	for _, q := range []string{"?status=paid", "?customer_id=x", "?from=yesterday", "?from=2026-03-05&to=2026-03-01", "?page=0", "?page_size=101", "?page=x"} {
		e.wantProblem(e.do("GET", "/v1/appointments"+q, e.staffTk, nil), 422, "validation_failed")
	}
	e.wantProblem(e.do("GET", "/v1/appointments/"+unknownID, e.staffTk, nil), 404, "appointment_not_found")
	if r := e.do("GET", "/v1/appointments/"+a1, e.staffTk, nil); r.status != 200 {
		t.Errorf("get: %d", r.status)
	}
}

// Over the HTTP layer, with the in-memory store: of N simultaneous requests for one
// slot exactly one is created and every other one gets 409 (the same property is proven
// against PostgreSQL in internal/app and internal/postgres).
func TestConcurrentBookingsOfOneSlot(t *testing.T) {
	e := newEnv(t)
	svc, cus := e.newService("Cut", 30, 5000), e.newCustomer(1)
	const n = 20
	statuses := make([]int, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			statuses[i] = e.bookAt(cus, svc, 2*time.Hour).status
		}()
	}
	close(start)
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
		t.Errorf("created = %d, conflicts = %d (statuses %v)", created, conflicts, statuses)
	}
}
