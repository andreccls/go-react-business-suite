// Package booking holds the appointment rules: no past start, no inactive service,
// price/duration snapshot, valid status transitions and — through the Repository
// contract — no overlapping appointments on the single agenda (ADR 0002).
package booking

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
	"github.com/andreccls/go-react-business-suite/backend/internal/period"
	"github.com/andreccls/go-react-business-suite/backend/internal/validation"
)

// Status is the lifecycle of an appointment: scheduled -> completed | cancelled | no_show.
type Status string

const (
	Scheduled Status = "scheduled"
	Completed Status = "completed"
	Cancelled Status = "cancelled"
	NoShow    Status = "no_show"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	return s == Scheduled || s == Completed || s == Cancelled || s == NoShow
}

// CanTransitionTo is the whole state machine: only scheduled appointments change,
// and the other three states are terminal.
func (s Status) CanTransitionTo(to Status) bool {
	return s == Scheduled && (to == Completed || to == Cancelled || to == NoShow)
}

// OccupiesAgenda says whether an appointment in this status blocks its time slot.
// Cancelled and no-show appointments free the slot. The PostgreSQL exclusion
// constraint uses the same two statuses (migration 0002).
func (s Status) OccupiesAgenda() bool { return s == Scheduled || s == Completed }

// Appointment is one booking. ServiceName, DurationMin and PriceCents are a SNAPSHOT
// of the service at booking time (ADR 0003); EndsAt is derived from the duration.
// CustomerName is a read-model field filled by the repository on reads.
type Appointment struct {
	ID           string    `json:"id"`
	CustomerID   string    `json:"customer_id"`
	CustomerName string    `json:"customer_name"`
	ServiceID    string    `json:"service_id"`
	ServiceName  string    `json:"service_name"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	DurationMin  int       `json:"duration_min"`
	PriceCents   int       `json:"price_cents"`
	Status       Status    `json:"status"`
	Notes        string    `json:"notes"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Input is what a client sends to book.
type Input struct {
	CustomerID string    `json:"customer_id"`
	ServiceID  string    `json:"service_id"`
	StartsAt   time.Time `json:"starts_at"`
	Notes      string    `json:"notes"`
}

// Query is the raw list filter (dates as sent by the client, business time zone).
type Query struct {
	Status     Status
	CustomerID string
	From, To   string // YYYY-MM-DD, inclusive, both optional
	Page       int
	PageSize   int
}

// Filter is the resolved filter handed to the Repository: [From, To) instants,
// zero = unbounded.
type Filter struct {
	Status     Status
	CustomerID string
	From, To   time.Time
	Page       int
	PageSize   int
}

// Domain errors.
var (
	ErrNotFound          = errors.New("appointment not found")
	ErrSlotTaken         = errors.New("time slot is not available")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrNotStarted        = errors.New("appointment has not started yet")
	ErrServiceInactive   = errors.New("service is inactive")
	ErrUnknownReference  = errors.New("unknown customer or service")
)

// Pagination bounds.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Repository is what the Service needs from storage.
type Repository interface {
	// Create stores a; ErrSlotTaken when it overlaps an appointment that occupies the
	// agenda (this check is atomic: concurrent creates for one slot yield exactly one
	// success); ErrUnknownReference when the customer or service does not exist.
	Create(ctx context.Context, a Appointment) error
	Get(ctx context.Context, id string) (Appointment, error) // ErrNotFound
	// List returns one page ordered by start time and the total number of matches.
	List(ctx context.Context, f Filter) ([]Appointment, int, error)
	// Upcoming returns up to limit scheduled appointments starting at or after now.
	Upcoming(ctx context.Context, now time.Time, limit int) ([]Appointment, error)
	// Transition moves id from -> to atomically (compare-and-set on the status).
	// ErrNotFound when absent; ErrInvalidTransition when the stored status is not `from`.
	Transition(ctx context.Context, id string, from, to Status, at time.Time) (Appointment, error)
}

// Catalog and Customers are the lookups the Service needs; the catalog and customer
// services satisfy them.
type (
	Catalog interface {
		Get(ctx context.Context, id string) (catalog.Item, error)
	}
	Customers interface {
		Get(ctx context.Context, id string) (customer.Customer, error)
	}
)

// Service holds the appointment use cases.
type Service struct {
	repo      Repository
	catalog   Catalog
	customers Customers
	loc       *time.Location
	now       func() time.Time
}

// NewService builds a Service. loc is the business time zone (date filters); now is
// injectable (nil = time.Now).
func NewService(repo Repository, cat Catalog, cust Customers, loc *time.Location, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, catalog: cat, customers: cust, loc: loc, now: now}
}

// Create books an appointment, snapshotting the service's name, duration and price.
func (s *Service) Create(ctx context.Context, in Input) (Appointment, error) {
	now := s.now().UTC().Truncate(time.Microsecond)
	var v validation.Errors
	in.Notes = strings.TrimSpace(in.Notes)
	if validation.RuneLen(in.Notes) > 500 {
		v.Add("notes", "must have at most 500 characters")
	}
	start := in.StartsAt.UTC().Truncate(time.Microsecond)
	switch {
	case in.StartsAt.IsZero():
		v.Add("starts_at", "is required (RFC 3339, e.g. 2026-10-07T14:00:00-03:00)")
	case !start.After(now):
		v.Add("starts_at", "must be in the future")
	}
	if _, err := uuid.Parse(in.CustomerID); err != nil {
		v.Add("customer_id", "must be a valid id")
	}
	if _, err := uuid.Parse(in.ServiceID); err != nil {
		v.Add("service_id", "must be a valid id")
	}
	if err := v.Err(); err != nil {
		return Appointment{}, err
	}

	cust, err := s.customers.Get(ctx, in.CustomerID)
	if errors.Is(err, customer.ErrNotFound) {
		v.Add("customer_id", "unknown customer")
		return Appointment{}, v.Err()
	}
	if err != nil {
		return Appointment{}, err
	}
	item, err := s.catalog.Get(ctx, in.ServiceID)
	if errors.Is(err, catalog.ErrNotFound) {
		v.Add("service_id", "unknown service")
		return Appointment{}, v.Err()
	}
	if err != nil {
		return Appointment{}, err
	}
	if !item.Active {
		return Appointment{}, ErrServiceInactive
	}

	a := Appointment{
		ID: uuid.NewString(), CustomerID: cust.ID, CustomerName: cust.Name,
		ServiceID: item.ID, ServiceName: item.Name,
		StartsAt: start, EndsAt: start.Add(time.Duration(item.DurationMin) * time.Minute),
		DurationMin: item.DurationMin, PriceCents: item.PriceCents,
		Status: Scheduled, Notes: in.Notes, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return Appointment{}, err
	}
	return a, nil
}

// Get returns one appointment.
func (s *Service) Get(ctx context.Context, id string) (Appointment, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Appointment{}, ErrNotFound
	}
	return s.repo.Get(ctx, id)
}

// List returns a page of appointments (start time order).
func (s *Service) List(ctx context.Context, q Query) ([]Appointment, int, error) {
	var v validation.Errors
	if q.Page < 1 {
		v.Add("page", "must be >= 1")
	}
	if q.PageSize < 1 || q.PageSize > MaxPageSize {
		v.Add("page_size", "must be between 1 and 100")
	}
	if q.Status != "" && !q.Status.Valid() {
		v.Add("status", "must be scheduled, completed, cancelled or no_show")
	}
	if q.CustomerID != "" {
		if _, err := uuid.Parse(q.CustomerID); err != nil {
			v.Add("customer_id", "must be a valid id")
		}
	}
	r, rerr := period.Parse(q.From, q.To, s.loc, s.now(), 0)
	if rerr != nil {
		var pe validation.Errors
		errors.As(rerr, &pe)
		v = append(v, pe...)
	}
	if err := v.Err(); err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, Filter{Status: q.Status, CustomerID: q.CustomerID, From: r.From, To: r.To, Page: q.Page, PageSize: q.PageSize})
}

// SetStatus applies a status transition. Completed and no_show need the appointment
// to have started; cancelled is allowed any time while scheduled.
func (s *Service) SetStatus(ctx context.Context, id string, to Status) (Appointment, error) {
	if !to.Valid() {
		var v validation.Errors
		v.Add("status", "must be completed, cancelled or no_show")
		return Appointment{}, v
	}
	a, err := s.Get(ctx, id)
	if err != nil {
		return Appointment{}, err
	}
	if !a.Status.CanTransitionTo(to) {
		return Appointment{}, ErrInvalidTransition
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	if to != Cancelled && now.Before(a.StartsAt) {
		return Appointment{}, ErrNotStarted
	}
	return s.repo.Transition(ctx, id, a.Status, to, now)
}
