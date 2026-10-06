package repotest

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
)

// Customers runs the customer.Repository contract.
func Customers(t *testing.T, newStores Factory) {
	t.Run("create then get round-trips every field; e-mail is unique", func(t *testing.T) {
		s := newStores(t)
		c := mustCustomer(t, s, "Ana Souza", base)
		got, err := s.Customers.Get(ctx, c.ID)
		if err != nil || got != c {
			t.Errorf("got %+v (%v), want %+v", got, err, c)
		}
		dup := c
		dup.ID = uuid.NewString()
		if err := s.Customers.Create(ctx, dup); !errors.Is(err, customer.ErrEmailTaken) {
			t.Errorf("duplicate e-mail: %v", err)
		}
		if _, err := s.Customers.Get(ctx, uuid.NewString()); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("missing: %v", err)
		}
	})

	t.Run("list: name order, search and pagination", func(t *testing.T) {
		s := newStores(t)
		mk := func(name, email, phone string) {
			c := customer.Customer{ID: uuid.NewString(), Name: name, Email: email, Phone: phone, CreatedAt: base, UpdatedAt: base}
			if err := s.Customers.Create(ctx, c); err != nil {
				t.Fatal(err)
			}
		}
		mk("Carla", "carla@mail.test", "31911112222")
		mk("Ana", "ana@mail.test", "31933334444")
		mk("Bruno", "bruno@other.test", "11955556666")

		list := func(q string, page, size int) ([]string, int) {
			items, total, err := s.Customers.List(ctx, customer.Filter{Query: q, Page: page, PageSize: size})
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, c := range items {
				names = append(names, c.Name)
			}
			return names, total
		}
		if got, total := list("", 1, 10); total != 3 || len(got) != 3 || got[0] != "Ana" || got[1] != "Bruno" || got[2] != "Carla" {
			t.Errorf("all = %v (%d)", got, total)
		}
		if got, total := list("MAIL.test", 1, 10); total != 2 || got[0] != "Ana" || got[1] != "Carla" {
			t.Errorf("by e-mail = %v (%d)", got, total)
		}
		if got, total := list("5555", 1, 10); total != 1 || got[0] != "Bruno" {
			t.Errorf("by phone = %v (%d)", got, total)
		}
		if got, total := list("_", 1, 10); total != 0 || len(got) != 0 {
			t.Errorf("a wildcard character must match literally: %v (%d)", got, total)
		}
		if got, total := list("", 2, 2); total != 3 || len(got) != 1 || got[0] != "Carla" {
			t.Errorf("page 2 = %v (%d)", got, total)
		}
		if items, _, err := s.Customers.List(ctx, customer.Filter{Page: 5, PageSize: 2}); err != nil || items == nil || len(items) != 0 {
			t.Errorf("beyond the last page = %v, %v", items, err)
		}
	})

	t.Run("update: e-mail conflicts and missing rows", func(t *testing.T) {
		s := newStores(t)
		a := mustCustomer(t, s, "Ana", base)
		b := mustCustomer(t, s, "Bia", base)
		b.Email = a.Email
		if err := s.Customers.Update(ctx, b); !errors.Is(err, customer.ErrEmailTaken) {
			t.Errorf("conflict: %v", err)
		}
		a.Name, a.Notes, a.UpdatedAt = "Ana M.", "vip", base.Add(time.Hour)
		if err := s.Customers.Update(ctx, a); err != nil {
			t.Fatalf("updating a row keeping its own e-mail: %v", err)
		}
		if got, _ := s.Customers.Get(ctx, a.ID); got != a {
			t.Errorf("after update: %+v", got)
		}
		ghost := a
		ghost.ID = uuid.NewString()
		if err := s.Customers.Update(ctx, ghost); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("update missing: %v", err)
		}
	})

	t.Run("delete: missing, in use, and success", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Cut", 30, 5000, true)
		withAppt := mustCustomer(t, s, "Ana", base)
		mustAppt(t, s, withAppt, it, base.Add(24*time.Hour), booking.Scheduled)
		free := mustCustomer(t, s, "Bia", base)

		if err := s.Customers.Delete(ctx, withAppt.ID); !errors.Is(err, customer.ErrInUse) {
			t.Errorf("in use: %v", err)
		}
		if err := s.Customers.Delete(ctx, free.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.Customers.Delete(ctx, free.ID); !errors.Is(err, customer.ErrNotFound) {
			t.Errorf("twice: %v", err)
		}
	})
}
