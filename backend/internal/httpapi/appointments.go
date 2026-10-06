package httpapi

import (
	"net/http"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
)

type statusRequest struct {
	Status booking.Status `json:"status"`
}

func (s *server) createAppointment(w http.ResponseWriter, r *http.Request) {
	var in booking.Input
	if !s.decode(w, r, &in) {
		return
	}
	a, err := s.Bookings.Create(r.Context(), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/appointments/"+a.ID)
	writeJSON(w, http.StatusCreated, a)
}

func (s *server) listAppointments(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r)
	f := booking.Query{
		Status: booking.Status(q.str("status")), CustomerID: q.str("customer_id"), From: q.str("from"), To: q.str("to"),
		Page: q.integer("page", 1), PageSize: q.integer("page_size", booking.DefaultPageSize),
	}
	if err := q.errs.Err(); err != nil {
		s.fail(w, r, err)
		return
	}
	items, total, err := s.Bookings.List(r.Context(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeList(w, items, f.Page, f.PageSize, total)
}

func (s *server) getAppointment(w http.ResponseWriter, r *http.Request) {
	a, err := s.Bookings.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *server) setAppointmentStatus(w http.ResponseWriter, r *http.Request) {
	var in statusRequest
	if !s.decode(w, r, &in) {
		return
	}
	a, err := s.Bookings.SetStatus(r.Context(), r.PathValue("id"), in.Status)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}
