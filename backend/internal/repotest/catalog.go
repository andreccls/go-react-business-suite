package repotest

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
)

// Catalog runs the catalog.Repository contract.
func Catalog(t *testing.T, newStores Factory) {
	t.Run("create then get round-trips every field", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Haircut", 45, 8000, true)
		got, err := s.Catalog.Get(ctx, it.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != it.ID || got.Name != "Haircut" || got.Description != it.Description || got.DurationMin != 45 ||
			got.PriceCents != 8000 || !got.Active || !got.CreatedAt.Equal(base) || !got.UpdatedAt.Equal(base) {
			t.Errorf("got %+v, want %+v", got, it)
		}
		if _, err := s.Catalog.Get(ctx, uuid.NewString()); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("missing: %v", err)
		}
	})

	t.Run("list: name order, filters, pagination and total", func(t *testing.T) {
		s := newStores(t)
		mustItem(t, s, "Massage", 60, 12000, true)
		mustItem(t, s, "Color", 90, 20000, false)
		mustItem(t, s, "Cut", 30, 5000, true)
		mustItem(t, s, "Beard trim", 15, 3000, true)

		names := func(f catalog.Filter) (out []string, total int) {
			f.Page, f.PageSize = max(f.Page, 1), max(f.PageSize, 100)
			items, total, err := s.Catalog.List(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range items {
				out = append(out, it.Name)
			}
			return out, total
		}
		eq := func(got, want []string) bool {
			if len(got) != len(want) {
				return false
			}
			for i := range got {
				if got[i] != want[i] {
					return false
				}
			}
			return true
		}
		yes, no := true, false
		if got, total := names(catalog.Filter{}); !eq(got, []string{"Beard trim", "Color", "Cut", "Massage"}) || total != 4 {
			t.Errorf("all = %v (%d)", got, total)
		}
		if got, total := names(catalog.Filter{Active: &yes}); !eq(got, []string{"Beard trim", "Cut", "Massage"}) || total != 3 {
			t.Errorf("active = %v (%d)", got, total)
		}
		if got, total := names(catalog.Filter{Active: &no}); !eq(got, []string{"Color"}) || total != 1 {
			t.Errorf("inactive = %v (%d)", got, total)
		}
		if got, total := names(catalog.Filter{Query: "CUT"}); !eq(got, []string{"Cut"}) || total != 1 {
			t.Errorf("query = %v (%d)", got, total)
		}
		if got, total := names(catalog.Filter{Query: "%"}); len(got) != 0 || total != 0 {
			t.Errorf("a wildcard character must match literally: %v (%d)", got, total)
		}
		page2, total, err := s.Catalog.List(ctx, catalog.Filter{Page: 2, PageSize: 3})
		if err != nil || total != 4 || len(page2) != 1 || page2[0].Name != "Massage" {
			t.Errorf("page 2 = %+v (%d), %v", page2, total, err)
		}
		beyond, total, err := s.Catalog.List(ctx, catalog.Filter{Page: 9, PageSize: 3})
		if err != nil || total != 4 || beyond == nil || len(beyond) != 0 {
			t.Errorf("beyond the last page = %+v (%d), %v", beyond, total, err)
		}
	})

	t.Run("update and delete", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Cut", 30, 5000, true)
		it.Name, it.PriceCents, it.Active, it.UpdatedAt = "Cut+", 5500, false, base.Add(time.Hour)
		if err := s.Catalog.Update(ctx, it); err != nil {
			t.Fatal(err)
		}
		got, _ := s.Catalog.Get(ctx, it.ID)
		if got.Name != "Cut+" || got.PriceCents != 5500 || got.Active || !got.UpdatedAt.Equal(base.Add(time.Hour)) {
			t.Errorf("after update: %+v", got)
		}
		ghost := it
		ghost.ID = uuid.NewString()
		if err := s.Catalog.Update(ctx, ghost); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("update missing: %v", err)
		}
		if err := s.Catalog.Delete(ctx, it.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Catalog.Get(ctx, it.ID); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("after delete: %v", err)
		}
		if err := s.Catalog.Delete(ctx, it.ID); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("delete twice: %v", err)
		}
	})

	t.Run("an item with appointments cannot be deleted", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Cut", 30, 5000, true)
		c := mustCustomer(t, s, "Ana", base)
		mustAppt(t, s, c, it, base.Add(24*time.Hour), booking.Scheduled)
		if err := s.Catalog.Delete(ctx, it.ID); !errors.Is(err, catalog.ErrInUse) {
			t.Errorf("err = %v, want ErrInUse", err)
		}
		if _, err := s.Catalog.Get(ctx, it.ID); err != nil {
			t.Errorf("item must survive: %v", err)
		}
	})
}
