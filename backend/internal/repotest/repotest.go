// Package repotest holds contract tests shared by every implementation of the storage
// interfaces (in-memory and PostgreSQL), so the fakes used by the unit tests cannot
// drift from the real thing. Each suite takes a factory that returns EMPTY stores.
package repotest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
	"github.com/andreccls/go-react-business-suite/backend/internal/dashboard"
)

// Stores is one empty instance of every repository, sharing one database.
type Stores struct {
	Catalog      catalog.Repository
	Customers    customer.Repository
	Appointments booking.Repository
	Dashboard    dashboard.Reader
}

// Factory returns fresh, empty Stores.
type Factory func(t *testing.T) Stores

var ctx = context.Background()

// base is a fixed Monday noon (UTC) all fixtures hang from.
var base = time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)

func mustItem(t *testing.T, s Stores, name string, durationMin, price int, active bool) catalog.Item {
	t.Helper()
	it := catalog.Item{
		ID: uuid.NewString(), Name: name, Description: "desc " + name, DurationMin: durationMin,
		PriceCents: price, Active: active, CreatedAt: base, UpdatedAt: base,
	}
	if err := s.Catalog.Create(ctx, it); err != nil {
		t.Fatalf("create item: %v", err)
	}
	return it
}

func mustCustomer(t *testing.T, s Stores, name string, at time.Time) customer.Customer {
	t.Helper()
	c := customer.Customer{
		ID: uuid.NewString(), Name: name, Email: fmt.Sprintf("%s@example.com", uuid.NewString()[:8]),
		Phone: "31999990000", Notes: "n", CreatedAt: at, UpdatedAt: at,
	}
	if err := s.Customers.Create(ctx, c); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	return c
}

func newAppt(c customer.Customer, it catalog.Item, start time.Time, status booking.Status) booking.Appointment {
	return booking.Appointment{
		ID: uuid.NewString(), CustomerID: c.ID, CustomerName: c.Name,
		ServiceID: it.ID, ServiceName: it.Name, StartsAt: start,
		EndsAt:      start.Add(time.Duration(it.DurationMin) * time.Minute),
		DurationMin: it.DurationMin, PriceCents: it.PriceCents, Status: status,
		Notes: "", CreatedAt: base, UpdatedAt: base,
	}
}

func mustAppt(t *testing.T, s Stores, c customer.Customer, it catalog.Item, start time.Time, status booking.Status) booking.Appointment {
	t.Helper()
	a := newAppt(c, it, start, status)
	if err := s.Appointments.Create(ctx, a); err != nil {
		t.Fatalf("create appointment at %s: %v", start, err)
	}
	return a
}
