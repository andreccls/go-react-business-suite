// Package catalog is the service catalog: what the studio sells (name, duration,
// price). It knows nothing about HTTP or SQL; storage is the small Repository interface.
package catalog

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/validation"
)

// Item is one bookable service. PriceCents is an integer amount of the minor currency
// unit (no floats for money).
type Item struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	DurationMin int       `json:"duration_min"`
	PriceCents  int       `json:"price_cents"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Input is the writable part of an item. Active is optional on create (default true).
type Input struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	DurationMin int    `json:"duration_min"`
	PriceCents  int    `json:"price_cents"`
	Active      *bool  `json:"active"`
}

// Patch is a partial update: nil fields are left untouched.
type Patch struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	DurationMin *int    `json:"duration_min"`
	PriceCents  *int    `json:"price_cents"`
	Active      *bool   `json:"active"`
}

// Filter selects a page of items (name order).
type Filter struct {
	Active   *bool  // nil = any
	Query    string // case-insensitive substring of the name
	Page     int    // 1-based
	PageSize int
}

// Domain errors.
var (
	ErrNotFound = errors.New("service not found")
	ErrInUse    = errors.New("service is referenced by appointments")
)

// Pagination bounds.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Repository is what the Service needs from storage.
type Repository interface {
	Create(ctx context.Context, it Item) error
	Get(ctx context.Context, id string) (Item, error) // ErrNotFound
	// List returns one page ordered by name and the total number of matches.
	List(ctx context.Context, f Filter) ([]Item, int, error)
	Update(ctx context.Context, it Item) error // ErrNotFound
	// Delete removes the item; ErrNotFound when absent, ErrInUse when appointments reference it.
	Delete(ctx context.Context, id string) error
}

// Service holds the catalog use cases.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService builds a Service. now is injectable (nil = time.Now).
func NewService(repo Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

// Create validates in and stores a new item.
func (s *Service) Create(ctx context.Context, in Input) (Item, error) {
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	n, err := normalize(in.Name, in.Description, in.DurationMin, in.PriceCents)
	if err != nil {
		return Item{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	it := Item{ID: uuid.NewString(), Active: active, CreatedAt: now, UpdatedAt: now}
	it.Name, it.Description, it.DurationMin, it.PriceCents = n.name, n.description, n.duration, n.price
	if err := s.repo.Create(ctx, it); err != nil {
		return Item{}, err
	}
	return it, nil
}

// Get returns one item.
func (s *Service) Get(ctx context.Context, id string) (Item, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Item{}, ErrNotFound
	}
	return s.repo.Get(ctx, id)
}

// List returns a page of items.
func (s *Service) List(ctx context.Context, f Filter) ([]Item, int, error) {
	var v validation.Errors
	if f.Page < 1 {
		v.Add("page", "must be >= 1")
	}
	if f.PageSize < 1 || f.PageSize > MaxPageSize {
		v.Add("page_size", "must be between 1 and 100")
	}
	if err := v.Err(); err != nil {
		return nil, 0, err
	}
	f.Query = strings.TrimSpace(f.Query)
	return s.repo.List(ctx, f)
}

// Update is the partial update (PATCH); the last write wins.
func (s *Service) Update(ctx context.Context, id string, p Patch) (Item, error) {
	it, err := s.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	name, desc, dur, price := it.Name, it.Description, it.DurationMin, it.PriceCents
	if p.Name != nil {
		name = *p.Name
	}
	if p.Description != nil {
		desc = *p.Description
	}
	if p.DurationMin != nil {
		dur = *p.DurationMin
	}
	if p.PriceCents != nil {
		price = *p.PriceCents
	}
	n, err := normalize(name, desc, dur, price)
	if err != nil {
		return Item{}, err
	}
	it.Name, it.Description, it.DurationMin, it.PriceCents = n.name, n.description, n.duration, n.price
	if p.Active != nil {
		it.Active = *p.Active
	}
	it.UpdatedAt = s.now().UTC().Truncate(time.Microsecond)
	if err := s.repo.Update(ctx, it); err != nil {
		return Item{}, err
	}
	return it, nil
}

// Delete removes an item that no appointment references (deactivate it otherwise).
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, id)
}

type normalized struct {
	name, description string
	duration, price   int
}

func normalize(name, desc string, duration, price int) (normalized, error) {
	var v validation.Errors
	name, desc = strings.TrimSpace(name), strings.TrimSpace(desc)
	if n := validation.RuneLen(name); n < 2 || n > 120 {
		v.Add("name", "must have between 2 and 120 characters")
	}
	if validation.RuneLen(desc) > 500 {
		v.Add("description", "must have at most 500 characters")
	}
	if duration < 5 || duration > 480 {
		v.Add("duration_min", "must be between 5 and 480 minutes")
	}
	if price < 0 || price > 100_000_00 {
		v.Add("price_cents", "must be between 0 and 10000000 (cents)")
	}
	return normalized{name, desc, duration, price}, v.Err()
}
