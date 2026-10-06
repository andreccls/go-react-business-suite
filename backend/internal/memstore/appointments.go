package memstore

import (
	"context"
	"time"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
)

// Appointments is an in-memory booking.Repository. The mutex makes the overlap check
// and the insert one atomic step, like the PostgreSQL exclusion constraint does.
type Appointments struct{ d *DB }

func (r *Appointments) withCustomer(a booking.Appointment) booking.Appointment {
	a.CustomerName = r.d.customers[a.CustomerID].Name
	return a
}

func (r *Appointments) Create(_ context.Context, a booking.Appointment) error {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	if _, ok := r.d.customers[a.CustomerID]; !ok {
		return booking.ErrUnknownReference
	}
	if _, ok := r.d.items[a.ServiceID]; !ok {
		return booking.ErrUnknownReference
	}
	if a.Status.OccupiesAgenda() {
		for _, o := range r.d.appointments {
			if o.Status.OccupiesAgenda() && a.StartsAt.Before(o.EndsAt) && o.StartsAt.Before(a.EndsAt) {
				return booking.ErrSlotTaken
			}
		}
	}
	r.d.appointments[a.ID] = a
	return nil
}

func (r *Appointments) Get(_ context.Context, id string) (booking.Appointment, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	a, ok := r.d.appointments[id]
	if !ok {
		return booking.Appointment{}, booking.ErrNotFound
	}
	return r.withCustomer(a), nil
}

func (r *Appointments) List(_ context.Context, f booking.Filter) ([]booking.Appointment, int, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []booking.Appointment
	for _, a := range r.d.appointments {
		switch {
		case f.Status != "" && a.Status != f.Status,
			f.CustomerID != "" && a.CustomerID != f.CustomerID,
			!f.From.IsZero() && a.StartsAt.Before(f.From),
			!f.To.IsZero() && !a.StartsAt.Before(f.To):
			continue
		}
		out = append(out, r.withCustomer(a))
	}
	sortBy(out, byStart)
	return page(out, f.Page, f.PageSize), len(out), nil
}

func byStart(a, b booking.Appointment) bool {
	if !a.StartsAt.Equal(b.StartsAt) {
		return a.StartsAt.Before(b.StartsAt)
	}
	return a.ID < b.ID
}

func (r *Appointments) Upcoming(_ context.Context, now time.Time, limit int) ([]booking.Appointment, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	out := []booking.Appointment{}
	for _, a := range r.d.appointments {
		if a.Status == booking.Scheduled && !a.StartsAt.Before(now) {
			out = append(out, r.withCustomer(a))
		}
	}
	sortBy(out, byStart)
	return out[:min(limit, len(out))], nil
}

func (r *Appointments) Transition(_ context.Context, id string, from, to booking.Status, at time.Time) (booking.Appointment, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	a, ok := r.d.appointments[id]
	if !ok {
		return booking.Appointment{}, booking.ErrNotFound
	}
	if a.Status != from {
		return booking.Appointment{}, booking.ErrInvalidTransition
	}
	a.Status, a.UpdatedAt = to, at
	r.d.appointments[id] = a
	return r.withCustomer(a), nil
}
