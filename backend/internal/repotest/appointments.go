package repotest

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
)

// Appointments runs the booking.Repository contract, including the no-overlap rule
// and its behaviour under concurrency.
func Appointments(t *testing.T, newStores Factory) {
	day := base.Add(24 * time.Hour) // a Tuesday noon
	at := func(h, m int) time.Time { return time.Date(2026, 3, 3, h, m, 0, 0, time.UTC) }

	t.Run("create then get round-trips every field and fills the customer name", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Cut", 30, 5000, true)
		c := mustCustomer(t, s, "Ana Souza", base)
		a := newAppt(c, it, day, booking.Scheduled)
		a.Notes = "first visit"
		if err := s.Appointments.Create(ctx, a); err != nil {
			t.Fatal(err)
		}
		got, err := s.Appointments.Get(ctx, a.ID)
		if err != nil || got != a {
			t.Errorf("got %+v (%v)\nwant %+v", got, err, a)
		}
		if got.CustomerName != "Ana Souza" {
			t.Errorf("customer name = %q", got.CustomerName)
		}
		if _, err := s.Appointments.Get(ctx, uuid.NewString()); !errors.Is(err, booking.ErrNotFound) {
			t.Errorf("missing: %v", err)
		}
	})

	t.Run("unknown customer or service is rejected", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Cut", 30, 5000, true)
		c := mustCustomer(t, s, "Ana", base)
		ghostC, ghostI := c, it
		ghostC.ID, ghostI.ID = uuid.NewString(), uuid.NewString()
		for name, a := range map[string]booking.Appointment{
			"customer": newAppt(ghostC, it, day, booking.Scheduled),
			"service":  newAppt(c, ghostI, day, booking.Scheduled),
		} {
			if err := s.Appointments.Create(ctx, a); !errors.Is(err, booking.ErrUnknownReference) {
				t.Errorf("unknown %s: %v", name, err)
			}
		}
	})

	t.Run("overlap rules (half-open intervals)", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Hour", 60, 10000, true) // 60 min
		half := mustItem(t, s, "Half", 30, 5000, true)
		c := mustCustomer(t, s, "Ana", base)
		mustAppt(t, s, c, it, at(10, 0), booking.Scheduled) // occupies [10:00, 11:00)

		cases := []struct {
			name    string
			item    catalog.Item
			start   time.Time
			wantErr error
		}{
			{"identical", it, at(10, 0), booking.ErrSlotTaken},
			{"starts inside", half, at(10, 30), booking.ErrSlotTaken},
			{"starts before, ends inside", it, at(9, 30), booking.ErrSlotTaken},
			{"contains it", it, at(9, 59), booking.ErrSlotTaken},
			{"contained", half, at(10, 15), booking.ErrSlotTaken},
			{"back-to-back after", half, at(11, 0), nil},
			{"back-to-back before", half, at(9, 30), nil},
			{"another day", it, at(10, 0).Add(24 * time.Hour), nil},
		}
		for _, tc := range cases {
			err := s.Appointments.Create(ctx, newAppt(c, tc.item, tc.start, booking.Scheduled))
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("%s: err = %v, want %v", tc.name, err, tc.wantErr)
			}
		}
	})

	t.Run("cancelled and no-show free the slot; completed still blocks", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Hour", 60, 10000, true)
		c := mustCustomer(t, s, "Ana", base)
		for _, st := range []booking.Status{booking.Cancelled, booking.NoShow} {
			mustAppt(t, s, c, it, at(10, 0), st)
			mustAppt(t, s, c, it, at(10, 0), st) // two dead ones in the same slot: fine
		}
		live := mustAppt(t, s, c, it, at(10, 0), booking.Scheduled) // the slot is free for a live one
		if err := s.Appointments.Create(ctx, newAppt(c, it, at(10, 0), booking.Scheduled)); !errors.Is(err, booking.ErrSlotTaken) {
			t.Errorf("second live one: %v", err)
		}
		// completing keeps the slot; cancelling frees it again.
		if _, err := s.Appointments.Transition(ctx, live.ID, booking.Scheduled, booking.Completed, base); err != nil {
			t.Fatal(err)
		}
		if err := s.Appointments.Create(ctx, newAppt(c, it, at(10, 0), booking.Scheduled)); !errors.Is(err, booking.ErrSlotTaken) {
			t.Errorf("completed must keep blocking: %v", err)
		}
		other := mustAppt(t, s, c, it, at(14, 0), booking.Scheduled)
		if _, err := s.Appointments.Transition(ctx, other.ID, booking.Scheduled, booking.Cancelled, base); err != nil {
			t.Fatal(err)
		}
		mustAppt(t, s, c, it, at(14, 0), booking.Scheduled)
	})

	t.Run("transition is a compare-and-set", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Cut", 30, 5000, true)
		c := mustCustomer(t, s, "Ana", base)
		a := mustAppt(t, s, c, it, at(10, 0), booking.Scheduled)
		when := base.Add(48 * time.Hour)
		got, err := s.Appointments.Transition(ctx, a.ID, booking.Scheduled, booking.NoShow, when)
		if err != nil || got.Status != booking.NoShow || !got.UpdatedAt.Equal(when) || got.CustomerName != "Ana" || got.PriceCents != 5000 {
			t.Errorf("got %+v, %v", got, err)
		}
		if _, err := s.Appointments.Transition(ctx, a.ID, booking.Scheduled, booking.Completed, when); !errors.Is(err, booking.ErrInvalidTransition) {
			t.Errorf("stale `from`: %v", err)
		}
		if _, err := s.Appointments.Transition(ctx, uuid.NewString(), booking.Scheduled, booking.Completed, when); !errors.Is(err, booking.ErrNotFound) {
			t.Errorf("missing: %v", err)
		}
	})

	t.Run("list: filters, start order, pagination", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Cut", 30, 5000, true)
		ana, bia := mustCustomer(t, s, "Ana", base), mustCustomer(t, s, "Bia", base)
		a3 := mustAppt(t, s, ana, it, at(15, 0), booking.Scheduled)
		a1 := mustAppt(t, s, bia, it, at(9, 0), booking.Completed)
		a2 := mustAppt(t, s, ana, it, at(11, 0), booking.Cancelled)
		next := mustAppt(t, s, ana, it, at(9, 0).Add(48*time.Hour), booking.Scheduled)

		ids := func(f booking.Filter) ([]string, int) {
			f.Page, f.PageSize = max(f.Page, 1), max(f.PageSize, 100)
			items, total, err := s.Appointments.List(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			var out []string
			for _, a := range items {
				out = append(out, a.ID)
			}
			return out, total
		}
		same := func(got []string, want ...booking.Appointment) bool {
			if len(got) != len(want) {
				return false
			}
			for i := range want {
				if got[i] != want[i].ID {
					return false
				}
			}
			return true
		}
		if got, total := ids(booking.Filter{}); !same(got, a1, a2, a3, next) || total != 4 {
			t.Errorf("all (start order) = %v (%d)", got, total)
		}
		if got, _ := ids(booking.Filter{Status: booking.Scheduled}); !same(got, a3, next) {
			t.Errorf("status = %v", got)
		}
		if got, _ := ids(booking.Filter{CustomerID: bia.ID}); !same(got, a1) {
			t.Errorf("customer = %v", got)
		}
		// [from, to) is half-open on the start instant.
		if got, _ := ids(booking.Filter{From: at(11, 0), To: at(15, 0)}); !same(got, a2) {
			t.Errorf("range [11:00,15:00) = %v", got)
		}
		if got, _ := ids(booking.Filter{From: at(15, 0)}); !same(got, a3, next) {
			t.Errorf("from only = %v", got)
		}
		if got, _ := ids(booking.Filter{To: at(11, 0)}); !same(got, a1) {
			t.Errorf("to only = %v", got)
		}
		page, total, err := s.Appointments.List(ctx, booking.Filter{Page: 2, PageSize: 3})
		if err != nil || total != 4 || len(page) != 1 || page[0].ID != next.ID {
			t.Errorf("page 2 = %+v (%d), %v", page, total, err)
		}
		if page, _, _ := s.Appointments.List(ctx, booking.Filter{Page: 9, PageSize: 3}); page == nil || len(page) != 0 {
			t.Errorf("beyond the last page = %v", page)
		}
	})

	t.Run("upcoming: only scheduled, from now on, soonest first, limited", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Cut", 30, 5000, true)
		c := mustCustomer(t, s, "Ana", base)
		mustAppt(t, s, c, it, at(8, 0), booking.Scheduled) // before "now"
		mustAppt(t, s, c, it, at(13, 0), booking.Cancelled)
		mustAppt(t, s, c, it, at(13, 0), booking.Completed)
		soon := mustAppt(t, s, c, it, at(12, 0), booking.Scheduled) // exactly now: included
		later := mustAppt(t, s, c, it, at(16, 0), booking.Scheduled)
		mustAppt(t, s, c, it, at(18, 0), booking.Scheduled)
		got, err := s.Appointments.Upcoming(ctx, at(12, 0), 2)
		if err != nil || len(got) != 2 || got[0].ID != soon.ID || got[1].ID != later.ID || got[0].CustomerName != "Ana" {
			t.Errorf("got %+v, %v", got, err)
		}
		if got, _ := s.Appointments.Upcoming(ctx, at(23, 0), 5); got == nil || len(got) != 0 {
			t.Errorf("nothing upcoming = %v", got)
		}
	})

	t.Run("concurrency: N creates for the same slot, exactly one wins", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Hour", 60, 10000, true)
		c := mustCustomer(t, s, "Ana", base)
		const n = 25
		errs := make([]error, n)
		var ready, wg sync.WaitGroup
		start := make(chan struct{})
		ready.Add(n)
		wg.Add(n)
		for i := range n {
			go func() {
				defer wg.Done()
				ready.Done()
				<-start
				errs[i] = s.Appointments.Create(ctx, newAppt(c, it, at(10, 0), booking.Scheduled))
			}()
		}
		ready.Wait()
		close(start)
		wg.Wait()

		wins, taken := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				wins++
			case errors.Is(err, booking.ErrSlotTaken):
				taken++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}
		if wins != 1 || taken != n-1 {
			t.Fatalf("wins = %d, slot taken = %d (want 1 and %d)", wins, taken, n-1)
		}
		if _, total, _ := s.Appointments.List(ctx, booking.Filter{Page: 1, PageSize: 100}); total != 1 {
			t.Errorf("stored appointments = %d, want 1", total)
		}
	})

	t.Run("concurrency: staggered overlapping creates never leave two overlapping rows", func(t *testing.T) {
		s := newStores(t)
		it := mustItem(t, s, "Hour", 60, 10000, true)
		c := mustCustomer(t, s, "Ana", base)
		const n = 24 // starts every 15 min: any two within 60 min overlap
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_ = s.Appointments.Create(ctx, newAppt(c, it, at(0, 0).Add(time.Duration(i)*15*time.Minute), booking.Scheduled))
			}()
		}
		close(start)
		wg.Wait()

		all, _, err := s.Appointments.List(ctx, booking.Filter{Page: 1, PageSize: 100})
		if err != nil || len(all) == 0 {
			t.Fatalf("list: %v (%d rows)", err, len(all))
		}
		for i := range all {
			for j := i + 1; j < len(all); j++ {
				if all[i].StartsAt.Before(all[j].EndsAt) && all[j].StartsAt.Before(all[i].EndsAt) {
					t.Fatalf("overlapping rows stored: %s and %s", all[i].StartsAt, all[j].StartsAt)
				}
			}
		}
		t.Logf("%d of %d staggered creates succeeded", len(all), n)
	})
}
