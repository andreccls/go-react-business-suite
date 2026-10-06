// Package customer is the customer registry (name, unique e-mail, phone, notes).
package customer

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/validation"
)

// Customer is the stored entity. Email is held lower-case, Phone digits-only.
type Customer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Input is the writable part of a customer.
type Input struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
	Notes string `json:"notes"`
}

// Patch is a partial update: nil fields are left untouched.
type Patch struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
	Phone *string `json:"phone"`
	Notes *string `json:"notes"`
}

// Filter selects a page of customers (name order).
type Filter struct {
	Query    string // case-insensitive substring of name, e-mail or phone
	Page     int    // 1-based
	PageSize int
}

// Domain errors.
var (
	ErrNotFound   = errors.New("customer not found")
	ErrEmailTaken = errors.New("e-mail already registered")
	ErrInUse      = errors.New("customer has appointments")
)

// Pagination bounds.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Repository is what the Service needs from storage.
type Repository interface {
	Create(ctx context.Context, c Customer) error // ErrEmailTaken
	Get(ctx context.Context, id string) (Customer, error)
	// List returns one page ordered by name and the total number of matches.
	List(ctx context.Context, f Filter) ([]Customer, int, error)
	Update(ctx context.Context, c Customer) error // ErrNotFound, ErrEmailTaken
	// Delete: ErrNotFound when absent, ErrInUse when appointments reference the customer.
	Delete(ctx context.Context, id string) error
}

// Service holds the customer use cases.
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

// Create validates in and stores a new customer.
func (s *Service) Create(ctx context.Context, in Input) (Customer, error) {
	n, err := normalize(in)
	if err != nil {
		return Customer{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	c := Customer{ID: uuid.NewString(), CreatedAt: now, UpdatedAt: now}
	c.Name, c.Email, c.Phone, c.Notes = n.Name, n.Email, n.Phone, n.Notes
	if err := s.repo.Create(ctx, c); err != nil {
		return Customer{}, err
	}
	return c, nil
}

// Get returns one customer.
func (s *Service) Get(ctx context.Context, id string) (Customer, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Customer{}, ErrNotFound
	}
	return s.repo.Get(ctx, id)
}

// List returns a page of customers.
func (s *Service) List(ctx context.Context, f Filter) ([]Customer, int, error) {
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
func (s *Service) Update(ctx context.Context, id string, p Patch) (Customer, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return Customer{}, err
	}
	m := Input{Name: c.Name, Email: c.Email, Phone: c.Phone, Notes: c.Notes}
	if p.Name != nil {
		m.Name = *p.Name
	}
	if p.Email != nil {
		m.Email = *p.Email
	}
	if p.Phone != nil {
		m.Phone = *p.Phone
	}
	if p.Notes != nil {
		m.Notes = *p.Notes
	}
	n, err := normalize(m)
	if err != nil {
		return Customer{}, err
	}
	c.Name, c.Email, c.Phone, c.Notes = n.Name, n.Email, n.Phone, n.Notes
	c.UpdatedAt = s.now().UTC().Truncate(time.Microsecond)
	if err := s.repo.Update(ctx, c); err != nil {
		return Customer{}, err
	}
	return c, nil
}

// Delete removes a customer that has no appointments.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, id)
}

func normalize(in Input) (Input, error) {
	var v validation.Errors
	in.Name = strings.TrimSpace(in.Name)
	if n := validation.RuneLen(in.Name); n < 2 || n > 120 {
		v.Add("name", "must have between 2 and 120 characters")
	}
	email, ok := validation.Email(in.Email)
	if !ok {
		v.Add("email", "must be a valid e-mail address")
	}
	in.Email = email
	if in.Phone = strings.TrimSpace(in.Phone); in.Phone != "" {
		p := validation.Digits(in.Phone)
		if strings.Trim(in.Phone, "0123456789()+- ") != "" || len(p) < 10 || len(p) > 13 {
			v.Add("phone", "must have 10 to 13 digits")
		}
		in.Phone = p
	}
	if in.Notes = strings.TrimSpace(in.Notes); validation.RuneLen(in.Notes) > 1000 {
		v.Add("notes", "must have at most 1000 characters")
	}
	return in, v.Err()
}
