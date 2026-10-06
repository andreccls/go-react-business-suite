package catalog_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/memstore"
	"github.com/andreccls/go-react-business-suite/backend/internal/validation"
)

var (
	ctx   = context.Background()
	t0    = time.Date(2026, 3, 1, 10, 0, 0, 123456789, time.UTC)
	boom  = errors.New("boom")
	truth = true
	lie   = false
)

func newService() *catalog.Service {
	return catalog.NewService(memstore.New().Catalog(), func() time.Time { return t0 })
}

// failing/updateFails make one write fail, to exercise the error paths.
type failing struct{ catalog.Repository }

func (failing) Create(context.Context, catalog.Item) error { return boom }

func fields(err error) []string {
	var v validation.Errors
	if !errors.As(err, &v) {
		return nil
	}
	var out []string
	for _, f := range v {
		out = append(out, f.Field)
	}
	return out
}

func TestCreate(t *testing.T) {
	s := newService()
	it, err := s.Create(ctx, catalog.Input{Name: "  Deep tissue massage ", Description: " 60 min ", DurationMin: 60, PriceCents: 12000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(it.ID); err != nil || it.Name != "Deep tissue massage" || it.Description != "60 min" ||
		!it.Active || it.DurationMin != 60 || it.PriceCents != 12000 {
		t.Errorf("unexpected item: %+v", it)
	}
	if !it.CreatedAt.Equal(t0.Truncate(time.Microsecond)) || !it.UpdatedAt.Equal(it.CreatedAt) {
		t.Errorf("timestamps = %v / %v (must be UTC, microsecond precision)", it.CreatedAt, it.UpdatedAt)
	}
	got, err := s.Get(ctx, it.ID)
	if err != nil || got != it {
		t.Errorf("stored %+v (%v)", got, err)
	}
	off, _ := s.Create(ctx, catalog.Input{Name: "Seasonal", DurationMin: 30, PriceCents: 0, Active: &lie})
	if off.Active {
		t.Error("explicit active=false must be kept")
	}
	on, _ := s.Create(ctx, catalog.Input{Name: "Regular", DurationMin: 30, PriceCents: 1, Active: &truth})
	if !on.Active {
		t.Error("explicit active=true must be kept")
	}
}

func TestCreateValidation(t *testing.T) {
	long := strings.Repeat("x", 501)
	tests := []struct {
		name string
		in   catalog.Input
		want []string
	}{
		{"all wrong", catalog.Input{Name: "x", Description: long, DurationMin: 4, PriceCents: -1}, []string{"name", "description", "duration_min", "price_cents"}},
		{"name too long", catalog.Input{Name: strings.Repeat("n", 121), DurationMin: 30}, []string{"name"}},
		{"duration too long", catalog.Input{Name: "ok", DurationMin: 481}, []string{"duration_min"}},
		{"price too high", catalog.Input{Name: "ok", DurationMin: 30, PriceCents: 10_000_001}, []string{"price_cents"}},
		{"blank name", catalog.Input{Name: "   ", DurationMin: 30}, []string{"name"}},
		{"edges are valid", catalog.Input{Name: "ok", DurationMin: 5, PriceCents: 0}, nil},
		{"upper edges are valid", catalog.Input{Name: "ok", DurationMin: 480, PriceCents: 10_000_000}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newService().Create(ctx, tt.in)
			got := fields(err)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("fields = %v, want %v (err %v)", got, tt.want, err)
			}
		})
	}
	if _, err := catalog.NewService(failing{memstore.New().Catalog()}, nil).Create(ctx, catalog.Input{Name: "ok", DurationMin: 30}); !errors.Is(err, boom) {
		t.Errorf("repository error not propagated: %v", err)
	}
}

func TestGetAndDeleteWithMalformedID(t *testing.T) {
	s := newService()
	if _, err := s.Get(ctx, "not-a-uuid"); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("Get: %v", err)
	}
	if err := s.Delete(ctx, "not-a-uuid"); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("Delete: %v", err)
	}
	if err := s.Delete(ctx, uuid.NewString()); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("Delete unknown: %v", err)
	}
	it, _ := s.Create(ctx, catalog.Input{Name: "Cut", DurationMin: 30})
	if err := s.Delete(ctx, it.ID); err != nil {
		t.Errorf("Delete: %v", err)
	}
}

func TestList(t *testing.T) {
	s := newService()
	for _, n := range []string{"Cut", "Color", "Massage"} {
		if _, err := s.Create(ctx, catalog.Input{Name: n, DurationMin: 30}); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := s.List(ctx, catalog.Filter{Query: "  CO ", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].Name != "Color" {
		t.Errorf("trimmed query: %+v (%d), %v", items, total, err)
	}
	for name, f := range map[string]catalog.Filter{
		"page 0":       {Page: 0, PageSize: 10},
		"size 0":       {Page: 1, PageSize: 0},
		"size too big": {Page: 1, PageSize: catalog.MaxPageSize + 1},
	} {
		if _, _, err := s.List(ctx, f); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
	if _, _, err := s.List(ctx, catalog.Filter{Page: 1, PageSize: catalog.MaxPageSize}); err != nil {
		t.Errorf("max page size must be accepted: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	later := t0.Add(time.Hour)
	now := t0
	s := catalog.NewService(memstore.New().Catalog(), func() time.Time { return now })
	it, _ := s.Create(ctx, catalog.Input{Name: "Cut", Description: "d", DurationMin: 30, PriceCents: 5000})

	now = later
	name, desc, dur, price := " Cut Pro ", "new", 45, 6500
	got, err := s.Update(ctx, it.ID, catalog.Patch{Name: &name, Description: &desc, DurationMin: &dur, PriceCents: &price, Active: &lie})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Cut Pro" || got.Description != "new" || got.DurationMin != 45 || got.PriceCents != 6500 || got.Active ||
		!got.CreatedAt.Equal(it.CreatedAt) || !got.UpdatedAt.Equal(later.Truncate(time.Microsecond)) {
		t.Errorf("after update: %+v", got)
	}
	// An empty patch changes nothing but the timestamp; only active can be flipped back.
	again, err := s.Update(ctx, it.ID, catalog.Patch{Active: &truth})
	if err != nil || again.Name != "Cut Pro" || !again.Active {
		t.Errorf("empty patch: %+v, %v", again, err)
	}

	bad := "x"
	if _, err := s.Update(ctx, it.ID, catalog.Patch{Name: &bad}); len(fields(err)) != 1 {
		t.Errorf("invalid patch: %v", err)
	}
	if stored, _ := s.Get(ctx, it.ID); stored.Name != "Cut Pro" {
		t.Error("an invalid patch must not be stored")
	}
	if _, err := s.Update(ctx, uuid.NewString(), catalog.Patch{}); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}

	repo := memstore.New().Catalog()
	_ = repo.Create(ctx, catalog.Item{ID: "6f1b1c3e-0b0e-4a53-9a43-3f0f0c1d2e3f", Name: "Cut", DurationMin: 30})
	if _, err := catalog.NewService(updateFails{repo}, nil).Update(ctx, "6f1b1c3e-0b0e-4a53-9a43-3f0f0c1d2e3f", catalog.Patch{}); !errors.Is(err, boom) {
		t.Errorf("repository error not propagated: %v", err)
	}
}

type updateFails struct{ catalog.Repository }

func (updateFails) Update(context.Context, catalog.Item) error { return boom }
