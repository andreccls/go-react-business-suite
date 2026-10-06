package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const unknownID = "6f1b1c3e-0b0e-4a53-9a43-3f0f0c1d2e3f"

func TestServiceLifecycle(t *testing.T) {
	e := newEnv(t)
	created := e.do("POST", "/v1/services", e.staffTk, map[string]any{"name": "  Deep tissue massage ", "description": "d", "duration_min": 60, "price_cents": 12000})
	if created.status != http.StatusCreated {
		t.Fatalf("create: %d %s", created.status, created.body)
	}
	s := created.json(t)
	id := s["id"].(string)
	if created.header.Get("Location") != "/v1/services/"+id || s["name"] != "Deep tissue massage" || s["active"] != true || s["price_cents"] != 12000.0 {
		t.Errorf("create response: %v %v", created.header, s)
	}
	if r := e.do("GET", "/v1/services/"+id, e.staffTk, nil); r.status != 200 || r.json(t)["duration_min"] != 60.0 {
		t.Errorf("get: %d %s", r.status, r.body)
	}
	if r := e.do("PATCH", "/v1/services/"+id, e.staffTk, map[string]any{"price_cents": 13000, "active": false}); r.status != 200 || r.json(t)["price_cents"] != 13000.0 || r.json(t)["active"] != false || r.json(t)["name"] != "Deep tissue massage" {
		t.Errorf("patch: %d %s", r.status, r.body)
	}
	e.wantProblem(e.do("PATCH", "/v1/services/"+id, e.staffTk, map[string]any{"duration_min": 1}), 422, "validation_failed")
	e.wantProblem(e.do("PATCH", "/v1/services/"+unknownID, e.staffTk, map[string]any{"name": "Ok name"}), 404, "service_not_found")

	// staff cannot delete; admin can; afterwards it is gone.
	e.wantProblem(e.do("DELETE", "/v1/services/"+id, e.staffTk, nil), 403, "forbidden")
	if r := e.do("DELETE", "/v1/services/"+id, e.adminTk, nil); r.status != 204 || len(r.body) != 0 {
		t.Errorf("delete: %d %s", r.status, r.body)
	}
	e.wantProblem(e.do("GET", "/v1/services/"+id, e.staffTk, nil), 404, "service_not_found")
	e.wantProblem(e.do("DELETE", "/v1/services/"+id, e.adminTk, nil), 404, "service_not_found")
	e.wantProblem(e.do("GET", "/v1/services/not-a-uuid", e.staffTk, nil), 404, "service_not_found")
}

func TestServiceValidationAndListing(t *testing.T) {
	e := newEnv(t)
	p := e.wantProblem(e.do("POST", "/v1/services", e.staffTk, map[string]any{"name": "x", "duration_min": 1, "price_cents": -5}), 422, "validation_failed")
	fields := map[string]bool{}
	for _, fe := range p["errors"].([]any) {
		fields[fe.(map[string]any)["field"].(string)] = true
	}
	if !fields["name"] || !fields["duration_min"] || !fields["price_cents"] {
		t.Errorf("errors = %v", p["errors"])
	}

	e.newService("Massage", 60, 12000)
	e.newService("Cut", 30, 5000)
	off := e.created("/v1/services", e.staffTk, map[string]any{"name": "Color", "duration_min": 90, "price_cents": 20000, "active": false})
	_ = off

	list := func(q string) map[string]any {
		t.Helper()
		r := e.do("GET", "/v1/services"+q, e.staffTk, nil)
		if r.status != 200 {
			t.Fatalf("list %s: %d %s", q, r.status, r.body)
		}
		return r.json(t)
	}
	names := func(m map[string]any) string {
		var out []string
		for _, it := range m["data"].([]any) {
			out = append(out, it.(map[string]any)["name"].(string))
		}
		return strings.Join(out, ",")
	}
	all := list("")
	if names(all) != "Color,Cut,Massage" || all["total"] != 3.0 || all["page"] != 1.0 || all["page_size"] != 20.0 {
		t.Errorf("defaults: %v", all)
	}
	if got := names(list("?active=true")); got != "Cut,Massage" {
		t.Errorf("active: %s", got)
	}
	if got := names(list("?active=false")); got != "Color" {
		t.Errorf("inactive: %s", got)
	}
	if got := names(list("?q=MASS")); got != "Massage" {
		t.Errorf("search: %s", got)
	}
	p2 := list("?page=2&page_size=2")
	if names(p2) != "Massage" || p2["total"] != 3.0 || p2["page"] != 2.0 || p2["page_size"] != 2.0 {
		t.Errorf("page 2: %v", p2)
	}
	if d, ok := list("?q=nobody")["data"].([]any); !ok || len(d) != 0 {
		t.Errorf("no matches must be [] not null")
	}
	for _, q := range []string{"?page=abc", "?page_size=1000", "?page=0", "?active=maybe", "?active=1", "?page_size=x"} {
		p := e.wantProblem(e.do("GET", "/v1/services"+q, e.staffTk, nil), 422, "validation_failed")
		if _, ok := p["errors"].([]any); !ok {
			t.Errorf("%s: missing errors list: %v", q, p)
		}
	}
}

func TestCustomerLifecycle(t *testing.T) {
	e := newEnv(t)
	created := e.do("POST", "/v1/customers", e.staffTk, map[string]any{"name": "Ana Souza", "email": " ANA@Example.com ", "phone": "(31) 99999-0000", "notes": "vip"})
	if created.status != http.StatusCreated {
		t.Fatalf("create: %d %s", created.status, created.body)
	}
	c := created.json(t)
	id := c["id"].(string)
	if created.header.Get("Location") != "/v1/customers/"+id || c["email"] != "ana@example.com" || c["phone"] != "31999990000" {
		t.Errorf("create response: %v %v", created.header, c)
	}
	if r := e.do("GET", "/v1/customers/"+id, e.staffTk, nil); r.status != 200 || r.json(t)["notes"] != "vip" {
		t.Errorf("get: %d %s", r.status, r.body)
	}
	if r := e.do("PATCH", "/v1/customers/"+id, e.staffTk, map[string]any{"notes": "allergic to lavender"}); r.status != 200 || r.json(t)["notes"] != "allergic to lavender" || r.json(t)["name"] != "Ana Souza" {
		t.Errorf("patch: %d %s", r.status, r.body)
	}
	e.wantProblem(e.do("PATCH", "/v1/customers/"+id, e.staffTk, map[string]any{"email": "nope"}), 422, "validation_failed")
	e.wantProblem(e.do("PATCH", "/v1/customers/"+unknownID, e.staffTk, map[string]any{"notes": "x"}), 404, "customer_not_found")

	e.wantProblem(e.do("DELETE", "/v1/customers/"+id, e.staffTk, nil), 403, "forbidden")
	if r := e.do("DELETE", "/v1/customers/"+id, e.adminTk, nil); r.status != 204 {
		t.Errorf("delete: %d %s", r.status, r.body)
	}
	e.wantProblem(e.do("GET", "/v1/customers/"+id, e.staffTk, nil), 404, "customer_not_found")
	e.wantProblem(e.do("DELETE", "/v1/customers/"+id, e.adminTk, nil), 404, "customer_not_found")
}

func TestCustomerValidationConflictsAndListing(t *testing.T) {
	e := newEnv(t)
	e.newCustomer(1)
	e.wantProblem(e.do("POST", "/v1/customers", e.staffTk, map[string]any{"name": "Dup", "email": "C1@example.com"}), 409, "email_taken")
	e.wantProblem(e.do("POST", "/v1/customers", e.staffTk, map[string]any{"name": "x", "email": "nope"}), 422, "validation_failed")
	other := e.newCustomer(2)
	e.wantProblem(e.do("PATCH", "/v1/customers/"+other["id"].(string), e.staffTk, map[string]any{"email": "c1@example.com"}), 409, "email_taken")

	r := e.do("GET", "/v1/customers?q=C2%40", e.staffTk, nil).json(t)
	if r["total"] != 1.0 || r["data"].([]any)[0].(map[string]any)["name"] != "Customer 2" {
		t.Errorf("search: %v", r)
	}
	if r := e.do("GET", "/v1/customers?page=2&page_size=1", e.staffTk, nil).json(t); r["total"] != 2.0 || r["page"] != 2.0 || len(r["data"].([]any)) != 1 {
		t.Errorf("page 2: %v", r)
	}
	e.wantProblem(e.do("GET", "/v1/customers?page=x", e.staffTk, nil), 422, "validation_failed")
}

func TestInUseResourcesCannotBeDeleted(t *testing.T) {
	e := newEnv(t)
	svc, cus := e.newService("Cut", 30, 5000), e.newCustomer(1)
	if r := e.bookAt(cus, svc, time.Hour); r.status != http.StatusCreated {
		t.Fatalf("book: %d %s", r.status, r.body)
	}
	e.wantProblem(e.do("DELETE", "/v1/services/"+svc["id"].(string), e.adminTk, nil), 409, "service_in_use")
	e.wantProblem(e.do("DELETE", "/v1/customers/"+cus["id"].(string), e.adminTk, nil), 409, "customer_in_use")
	// The advice in the error works: deactivate instead.
	if r := e.do("PATCH", "/v1/services/"+svc["id"].(string), e.staffTk, map[string]any{"active": false}); r.status != 200 {
		t.Errorf("deactivate: %d", r.status)
	}
}
