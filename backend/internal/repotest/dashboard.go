package repotest

import (
	"testing"
	"time"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/dashboard"
)

// Dashboard runs the dashboard.Reader contract on a small hand-computed data set.
func Dashboard(t *testing.T, newStores Factory) {
	sp, err := time.LoadLocation("America/Sao_Paulo") // UTC-3, no DST
	if err != nil {
		t.Fatal(err)
	}
	utc := func(d, h, m int) time.Time { return time.Date(2026, 3, d, h, m, 0, 0, time.UTC) }

	// Data set (all times UTC; local = UTC-3):
	//   Cut  R$50 (30 min)    Color R$200 (90 min)    Massage R$120 (60 min)
	//   03-02 13:00 Cut     completed   local 03-02
	//   03-02 15:00 Color   completed   local 03-02
	//   03-03 01:30 Massage completed   local 03-02 (22:30!)  <- zone boundary
	//   03-03 13:00 Cut     completed   local 03-03
	//   03-03 16:00 Cut     cancelled
	//   03-04 13:00 Massage no_show
	//   03-04 17:00 Cut     scheduled
	//   03-10 13:00 Cut     completed   (outside the queried range)
	s := newStores(t)
	cut := mustItem(t, s, "Cut", 30, 5000, true)
	color := mustItem(t, s, "Color", 90, 20000, true)
	massage := mustItem(t, s, "Massage", 60, 12000, true)
	mustCustomer(t, s, "Early", utc(1, 9, 0)) // before the range
	in1 := mustCustomer(t, s, "In 1", utc(2, 9, 0))
	mustCustomer(t, s, "In 2", utc(4, 23, 59))
	mustCustomer(t, s, "Edge (to is exclusive)", utc(5, 3, 0))
	for _, a := range []struct {
		it     catalog.Item
		start  time.Time
		status booking.Status
	}{
		{cut, utc(2, 13, 0), booking.Completed}, {color, utc(2, 15, 0), booking.Completed},
		{massage, utc(3, 1, 30), booking.Completed}, {cut, utc(3, 13, 0), booking.Completed},
		{cut, utc(3, 16, 0), booking.Cancelled}, {massage, utc(4, 13, 0), booking.NoShow},
		{cut, utc(4, 17, 0), booking.Scheduled}, {cut, utc(10, 13, 0), booking.Completed},
	} {
		mustAppt(t, s, in1, a.it, a.start, a.status)
	}

	from, to := utc(2, 3, 0), utc(5, 3, 0) // local days 03-02 .. 03-04 inclusive

	t.Run("status totals", func(t *testing.T) {
		got, err := s.Dashboard.StatusTotals(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		want := map[booking.Status]dashboard.StatusTotals{
			booking.Completed: {Count: 4, RevenueCents: 5000 + 20000 + 12000 + 5000},
			booking.Cancelled: {Count: 1, RevenueCents: 5000},
			booking.NoShow:    {Count: 1, RevenueCents: 12000},
			booking.Scheduled: {Count: 1, RevenueCents: 5000},
		}
		if len(got) != len(want) {
			t.Errorf("got %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %+v, want %+v", k, got[k], v)
			}
		}
		if empty, err := s.Dashboard.StatusTotals(ctx, utc(20, 0, 0), utc(21, 0, 0)); err != nil || len(empty) != 0 {
			t.Errorf("empty range = %v, %v", empty, err)
		}
	})

	t.Run("new customers: [from, to) on created_at", func(t *testing.T) {
		n, err := s.Dashboard.NewCustomers(ctx, from, to)
		if err != nil || n != 2 {
			t.Errorf("= %d, %v; want 2 (the one at `to` and the early one are out)", n, err)
		}
	})

	t.Run("days are bucketed in the business zone", func(t *testing.T) {
		got, err := s.Dashboard.Days(ctx, from, to, sp)
		if err != nil {
			t.Fatal(err)
		}
		want := []dashboard.Day{
			{Date: "2026-03-02", Appointments: 3, Completed: 3, RevenueCents: 5000 + 20000 + 12000},
			{Date: "2026-03-03", Appointments: 2, Completed: 1, RevenueCents: 5000},
			{Date: "2026-03-04", Appointments: 2, Completed: 0, RevenueCents: 0},
		}
		if len(got) != len(want) {
			t.Fatalf("got %+v", got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("day %d = %+v, want %+v", i, got[i], want[i])
			}
		}
		// The same data in UTC puts the 01:30 appointment on 03-03.
		gotUTC, _ := s.Dashboard.Days(ctx, from, to, time.UTC)
		if len(gotUTC) == 0 || gotUTC[0].Date != "2026-03-02" || gotUTC[0].Appointments != 2 {
			t.Errorf("UTC buckets = %+v", gotUTC)
		}
		if none, err := s.Dashboard.Days(ctx, utc(20, 0, 0), utc(21, 0, 0), sp); err != nil || none == nil || len(none) != 0 {
			t.Errorf("empty = %v, %v", none, err)
		}
	})

	t.Run("top services: revenue, then completed, then appointments, then name", func(t *testing.T) {
		got, err := s.Dashboard.TopServices(ctx, from, to, 10)
		if err != nil {
			t.Fatal(err)
		}
		want := []dashboard.TopService{
			{ServiceID: color.ID, Name: "Color", Appointments: 1, Completed: 1, RevenueCents: 20000},
			{ServiceID: massage.ID, Name: "Massage", Appointments: 2, Completed: 1, RevenueCents: 12000},
			{ServiceID: cut.ID, Name: "Cut", Appointments: 4, Completed: 2, RevenueCents: 10000},
		}
		if len(got) != 3 {
			t.Fatalf("got %+v", got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("rank %d = %+v, want %+v", i+1, got[i], want[i])
			}
		}
		if top1, _ := s.Dashboard.TopServices(ctx, from, to, 1); len(top1) != 1 || top1[0].Name != "Color" {
			t.Errorf("limit 1 = %+v", top1)
		}
		if none, err := s.Dashboard.TopServices(ctx, utc(20, 0, 0), utc(21, 0, 0), 5); err != nil || none == nil || len(none) != 0 {
			t.Errorf("empty = %v, %v", none, err)
		}
	})

	t.Run("top services: ties break by name", func(t *testing.T) {
		s := newStores(t)
		b := mustItem(t, s, "B", 30, 1000, true)
		a := mustItem(t, s, "A", 30, 1000, true)
		c := mustCustomer(t, s, "X", base)
		mustAppt(t, s, c, b, utc(2, 13, 0), booking.Completed)
		mustAppt(t, s, c, a, utc(2, 14, 0), booking.Completed)
		got, err := s.Dashboard.TopServices(ctx, from, to, 5)
		if err != nil || len(got) != 2 || got[0].Name != "A" || got[1].Name != "B" {
			t.Errorf("got %+v, %v", got, err)
		}
	})
}
