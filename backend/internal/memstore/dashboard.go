package memstore

import (
	"context"
	"time"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/dashboard"
)

// Dashboard is an in-memory dashboard.Reader.
type Dashboard struct{ d *DB }

// inRange selects the appointments that START in [from, to).
func (r *Dashboard) inRange(from, to time.Time) []booking.Appointment {
	var out []booking.Appointment
	for _, a := range r.d.appointments {
		if !a.StartsAt.Before(from) && a.StartsAt.Before(to) {
			out = append(out, a)
		}
	}
	return out
}

func (r *Dashboard) StatusTotals(_ context.Context, from, to time.Time) (map[booking.Status]dashboard.StatusTotals, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	out := map[booking.Status]dashboard.StatusTotals{}
	for _, a := range r.inRange(from, to) {
		t := out[a.Status]
		t.Count++
		t.RevenueCents += a.PriceCents
		out[a.Status] = t
	}
	return out, nil
}

func (r *Dashboard) NewCustomers(_ context.Context, from, to time.Time) (int, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	n := 0
	for _, c := range r.d.customers {
		if !c.CreatedAt.Before(from) && c.CreatedAt.Before(to) {
			n++
		}
	}
	return n, nil
}

func (r *Dashboard) Days(_ context.Context, from, to time.Time, loc *time.Location) ([]dashboard.Day, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	byDate := map[string]*dashboard.Day{}
	for _, a := range r.inRange(from, to) {
		key := a.StartsAt.In(loc).Format(time.DateOnly)
		d := byDate[key]
		if d == nil {
			d = &dashboard.Day{Date: key}
			byDate[key] = d
		}
		d.Appointments++
		if a.Status == booking.Completed {
			d.Completed++
			d.RevenueCents += a.PriceCents
		}
	}
	out := []dashboard.Day{}
	for _, d := range byDate {
		out = append(out, *d)
	}
	sortBy(out, func(a, b dashboard.Day) bool { return a.Date < b.Date })
	return out, nil
}

func (r *Dashboard) TopServices(_ context.Context, from, to time.Time, limit int) ([]dashboard.TopService, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	byID := map[string]*dashboard.TopService{}
	for _, a := range r.inRange(from, to) {
		s := byID[a.ServiceID]
		if s == nil {
			s = &dashboard.TopService{ServiceID: a.ServiceID, Name: r.d.items[a.ServiceID].Name}
			byID[a.ServiceID] = s
		}
		s.Appointments++
		if a.Status == booking.Completed {
			s.Completed++
			s.RevenueCents += a.PriceCents
		}
	}
	out := []dashboard.TopService{}
	for _, s := range byID {
		out = append(out, *s)
	}
	sortBy(out, func(a, b dashboard.TopService) bool {
		switch {
		case a.RevenueCents != b.RevenueCents:
			return a.RevenueCents > b.RevenueCents
		case a.Completed != b.Completed:
			return a.Completed > b.Completed
		case a.Appointments != b.Appointments:
			return a.Appointments > b.Appointments
		case a.Name != b.Name:
			return a.Name < b.Name
		}
		return a.ServiceID < b.ServiceID
	})
	return out[:min(limit, len(out))], nil
}
