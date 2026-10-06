package httpapi

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/andreccls/go-react-business-suite/backend/internal/validation"
)

// listResponse is the envelope of every paginated list.
type listResponse[T any] struct {
	Data     []T `json:"data"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

func writeList[T any](w http.ResponseWriter, items []T, page, size, total int) {
	if items == nil {
		items = []T{}
	}
	writeJSON(w, http.StatusOK, listResponse[T]{Data: items, Page: page, PageSize: size, Total: total})
}

// query reads typed query parameters, collecting every malformed one as a field error
// (the services then validate the ranges).
type query struct {
	v    url.Values
	errs validation.Errors
}

func newQuery(r *http.Request) *query { return &query{v: r.URL.Query()} }

func (q *query) str(name string) string { return q.v.Get(name) }

func (q *query) integer(name string, def int) int {
	s := q.v.Get(name)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		q.errs.Add(name, "must be an integer")
	}
	return n
}

// boolean returns nil when the parameter is absent.
func (q *query) boolean(name string) *bool {
	var b bool
	switch q.v.Get(name) {
	case "":
		return nil
	case "true":
		b = true
	case "false":
	default:
		q.errs.Add(name, "must be true or false")
	}
	return &b
}
