package httpapi

import (
	"net/http"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
)

func (s *server) dashboardSummary(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r)
	sum, err := s.Dashboard.Summary(r.Context(), q.str("from"), q.str("to"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *server) dashboardDaily(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r)
	d, err := s.Dashboard.Daily(r.Context(), q.str("from"), q.str("to"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *server) dashboardTopServices(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r)
	limit := q.integer("limit", 0)
	if err := q.errs.Err(); err != nil {
		s.fail(w, r, err)
		return
	}
	top, err := s.Dashboard.TopServices(r.Context(), q.str("from"), q.str("to"), limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, top)
}

func (s *server) dashboardUpcoming(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r)
	limit := q.integer("limit", 0)
	if err := q.errs.Err(); err != nil {
		s.fail(w, r, err)
		return
	}
	items, err := s.Dashboard.UpcomingAppointments(r.Context(), limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Data []booking.Appointment `json:"data"`
	}{items})
}
