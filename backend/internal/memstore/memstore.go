// Package memstore holds in-memory implementations of every storage interface. They
// are the fakes of the unit tests, and the SAME contract suites (internal/repotest)
// that run against them also run against PostgreSQL, so they cannot drift.
package memstore

import (
	"sort"
	"strings"
	"sync"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
)

// DB is the shared state behind the catalog, customer, booking and dashboard fakes
// (they need one another: a foreign key, the overlap rule, the aggregations).
// One mutex guards everything.
type DB struct {
	mu           sync.Mutex
	items        map[string]catalog.Item
	customers    map[string]customer.Customer
	appointments map[string]booking.Appointment
}

// New returns an empty DB.
func New() *DB {
	return &DB{
		items:        map[string]catalog.Item{},
		customers:    map[string]customer.Customer{},
		appointments: map[string]booking.Appointment{},
	}
}

// Catalog, Customers, Appointments and Dashboard are views over the same DB.
func (d *DB) Catalog() *Catalog           { return &Catalog{d} }
func (d *DB) Customers() *Customers       { return &Customers{d} }
func (d *DB) Appointments() *Appointments { return &Appointments{d} }
func (d *DB) Dashboard() *Dashboard       { return &Dashboard{d} }

// page slices items for a 1-based page; total is computed by the caller.
func page[T any](items []T, p, size int) []T {
	start := (p - 1) * size
	if start >= len(items) {
		return []T{}
	}
	return items[start:min(start+size, len(items))]
}

func contains(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func sortBy[T any](items []T, less func(a, b T) bool) {
	sort.Slice(items, func(i, j int) bool { return less(items[i], items[j]) })
}
