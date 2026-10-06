package httpapi

import (
	"net/http"

	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
)

func (s *server) createService(w http.ResponseWriter, r *http.Request) {
	var in catalog.Input
	if !s.decode(w, r, &in) {
		return
	}
	it, err := s.Catalog.Create(r.Context(), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/services/"+it.ID)
	writeJSON(w, http.StatusCreated, it)
}

func (s *server) listServices(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r)
	f := catalog.Filter{Active: q.boolean("active"), Query: q.str("q"), Page: q.integer("page", 1), PageSize: q.integer("page_size", catalog.DefaultPageSize)}
	if err := q.errs.Err(); err != nil {
		s.fail(w, r, err)
		return
	}
	items, total, err := s.Catalog.List(r.Context(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeList(w, items, f.Page, f.PageSize, total)
}

func (s *server) getService(w http.ResponseWriter, r *http.Request) {
	it, err := s.Catalog.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *server) patchService(w http.ResponseWriter, r *http.Request) {
	var p catalog.Patch
	if !s.decode(w, r, &p) {
		return
	}
	it, err := s.Catalog.Update(r.Context(), r.PathValue("id"), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *server) deleteService(w http.ResponseWriter, r *http.Request) {
	if err := s.Catalog.Delete(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
