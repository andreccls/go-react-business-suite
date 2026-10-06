package customer_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
	"github.com/andreccls/go-react-business-suite/backend/internal/memstore"
	"github.com/andreccls/go-react-business-suite/backend/internal/validation"
)

var (
	ctx     = context.Background()
	t0      = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	errBoom = errors.New("boom")
)

type writeFails struct {
	customer.Repository
	create, update bool
}

func (w writeFails) Create(ctx context.Context, c customer.Customer) error {
	if w.create {
		return errBoom
	}
	return w.Repository.Create(ctx, c)
}

func (w writeFails) Update(ctx context.Context, c customer.Customer) error {
	if w.update {
		return errBoom
	}
	return w.Repository.Update(ctx, c)
}

func newService() *customer.Service {
	return customer.NewService(memstore.New().Customers(), func() time.Time { return t0 })
}

func fields(err error) string {
	var v validation.Errors
	if !errors.As(err, &v) {
		return ""
	}
	var out []string
	for _, f := range v {
		out = append(out, f.Field)
	}
	return strings.Join(out, ",")
}

func TestCreateNormalizes(t *testing.T) {
	s := newService()
	c, err := s.Create(ctx, customer.Input{Name: "  Ana Souza ", Email: " Ana@Example.COM ", Phone: "(31) 99999-0000", Notes: " VIP "})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(c.ID); err != nil || c.Name != "Ana Souza" || c.Email != "ana@example.com" ||
		c.Phone != "31999990000" || c.Notes != "VIP" || !c.CreatedAt.Equal(t0) {
		t.Errorf("unexpected customer: %+v", c)
	}
	if _, err := s.Create(ctx, customer.Input{Name: "Ana Again", Email: "ANA@example.com"}); !errors.Is(err, customer.ErrEmailTaken) {
		t.Errorf("duplicate e-mail: %v", err)
	}
	empty, err := s.Create(ctx, customer.Input{Name: "No Phone", Email: "np@example.com"})
	if err != nil || empty.Phone != "" || empty.Notes != "" {
		t.Errorf("optional fields: %+v, %v", empty, err)
	}
	if _, err := customer.NewService(writeFails{Repository: memstore.New().Customers(), create: true}, nil).Create(ctx, customer.Input{Name: "Ana", Email: "a@example.com"}); !errors.Is(err, errBoom) {
		t.Errorf("repository error: %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	tests := []struct {
		name string
		in   customer.Input
		want string
	}{
		{"everything wrong", customer.Input{Name: "A", Email: "nope", Phone: "123", Notes: strings.Repeat("n", 1001)}, "name,email,phone,notes"},
		{"phone with letters", customer.Input{Name: "Ana", Email: "a@example.com", Phone: "31 9999-abcd"}, "phone"},
		{"phone too long", customer.Input{Name: "Ana", Email: "a@example.com", Phone: "12345678901234"}, "phone"},
		{"name too long", customer.Input{Name: strings.Repeat("a", 121), Email: "a@example.com"}, "name"},
		{"international phone is fine", customer.Input{Name: "Ana", Email: "a@example.com", Phone: "+55 31 99999-0000"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newService().Create(ctx, tt.in)
			if got := fields(err); got != tt.want {
				t.Errorf("fields = %q, want %q (%v)", got, tt.want, err)
			}
		})
	}
}

func TestGetListDelete(t *testing.T) {
	s := newService()
	if _, err := s.Get(ctx, "nope"); !errors.Is(err, customer.ErrNotFound) {
		t.Errorf("Get malformed: %v", err)
	}
	if err := s.Delete(ctx, "nope"); !errors.Is(err, customer.ErrNotFound) {
		t.Errorf("Delete malformed: %v", err)
	}
	a, _ := s.Create(ctx, customer.Input{Name: "Ana", Email: "ana@example.com"})
	_, _ = s.Create(ctx, customer.Input{Name: "Bruno", Email: "bruno@example.com"})

	items, total, err := s.List(ctx, customer.Filter{Query: " BRU ", Page: 1, PageSize: 10})
	if err != nil || total != 1 || items[0].Name != "Bruno" {
		t.Errorf("search: %+v (%d) %v", items, total, err)
	}
	for name, f := range map[string]customer.Filter{
		"page 0": {Page: 0, PageSize: 10}, "size 0": {Page: 1}, "size too big": {Page: 1, PageSize: 101},
	} {
		if _, _, err := s.List(ctx, f); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
	if err := s.Delete(ctx, a.ID); err != nil {
		t.Errorf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, a.ID); !errors.Is(err, customer.ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	now := t0
	s := customer.NewService(memstore.New().Customers(), func() time.Time { return now })
	a, _ := s.Create(ctx, customer.Input{Name: "Ana", Email: "ana@example.com", Phone: "31999990000", Notes: "n"})
	b, _ := s.Create(ctx, customer.Input{Name: "Bia", Email: "bia@example.com"})

	now = t0.Add(time.Hour)
	name, email, phone, notes := " Ana M. ", "ANA.M@example.com", "(11) 98888-7777", "vip"
	got, err := s.Update(ctx, a.ID, customer.Patch{Name: &name, Email: &email, Phone: &phone, Notes: &notes})
	if err != nil || got.Name != "Ana M." || got.Email != "ana.m@example.com" || got.Phone != "11988887777" || got.Notes != "vip" ||
		!got.CreatedAt.Equal(t0) || !got.UpdatedAt.Equal(now) {
		t.Errorf("after update: %+v, %v", got, err)
	}
	if same, err := s.Update(ctx, a.ID, customer.Patch{}); err != nil || same.Name != "Ana M." {
		t.Errorf("empty patch: %+v, %v", same, err)
	}
	if _, err := s.Update(ctx, a.ID, customer.Patch{Email: &b.Email}); !errors.Is(err, customer.ErrEmailTaken) {
		t.Errorf("e-mail of another customer: %v", err)
	}
	bad := "nope"
	if _, err := s.Update(ctx, a.ID, customer.Patch{Email: &bad}); fields(err) != "email" {
		t.Errorf("invalid patch: %v", err)
	}
	if _, err := s.Update(ctx, uuid.NewString(), customer.Patch{}); !errors.Is(err, customer.ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	failing := customer.NewService(writeFails{Repository: memstore.New().Customers(), update: true}, nil)
	c, _ := failing.Create(ctx, customer.Input{Name: "Cris", Email: "c@example.com"})
	if _, err := failing.Update(ctx, c.ID, customer.Patch{}); !errors.Is(err, errBoom) {
		t.Errorf("repository error: %v", err)
	}
}
