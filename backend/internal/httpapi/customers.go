package httpapi

import (
	"net/http"

	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
)

func (s *server) createCustomer(w http.ResponseWriter, r *http.Request) {
	var in customer.Input
	if !s.decode(w, r, &in) {
		return
	}
	c, err := s.Customers.Create(r.Context(), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/customers/"+c.ID)
	writeJSON(w, http.StatusCreated, c)
}

func (s *server) listCustomers(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r)
	f := customer.Filter{Query: q.str("q"), Page: q.integer("page", 1), PageSize: q.integer("page_size", customer.DefaultPageSize)}
	if err := q.errs.Err(); err != nil {
		s.fail(w, r, err)
		return
	}
	items, total, err := s.Customers.List(r.Context(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeList(w, items, f.Page, f.PageSize, total)
}

func (s *server) getCustomer(w http.ResponseWriter, r *http.Request) {
	c, err := s.Customers.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) patchCustomer(w http.ResponseWriter, r *http.Request) {
	var p customer.Patch
	if !s.decode(w, r, &p) {
		return
	}
	c, err := s.Customers.Update(r.Context(), r.PathValue("id"), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) deleteCustomer(w http.ResponseWriter, r *http.Request) {
	if err := s.Customers.Delete(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
