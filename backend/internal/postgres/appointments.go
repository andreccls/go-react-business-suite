package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
)

// Appointments is the PostgreSQL booking.Repository. The no-overlap rule is NOT checked
// here in Go: it is the appointments_no_overlap exclusion constraint (migration 0002),
// which PostgreSQL enforces atomically, so a check-then-insert race is impossible.
type Appointments struct{ pool *pgxpool.Pool }

// NewAppointments returns a repository on pool.
func NewAppointments(pool *pgxpool.Pool) *Appointments { return &Appointments{pool} }

const apptSelect = `SELECT a.id, a.customer_id, c.name, a.service_id, a.service_name, a.starts_at, a.ends_at,
	a.duration_min, a.price_cents, a.status, a.notes, a.created_at, a.updated_at
	FROM appointments a JOIN customers c ON c.id = a.customer_id`

func scanAppointment(r scanner) (booking.Appointment, error) {
	var a booking.Appointment
	var status string
	err := r.Scan(&a.ID, &a.CustomerID, &a.CustomerName, &a.ServiceID, &a.ServiceName, &a.StartsAt, &a.EndsAt,
		&a.DurationMin, &a.PriceCents, &status, &a.Notes, &a.CreatedAt, &a.UpdatedAt)
	a.Status = booking.Status(status)
	a.StartsAt, a.EndsAt, a.CreatedAt, a.UpdatedAt = a.StartsAt.UTC(), a.EndsAt.UTC(), a.CreatedAt.UTC(), a.UpdatedAt.UTC()
	return a, err
}

func (s *Appointments) Create(ctx context.Context, a booking.Appointment) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO appointments (id, customer_id, service_id, service_name, duration_min, price_cents,
			starts_at, ends_at, status, notes, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.ID, a.CustomerID, a.ServiceID, a.ServiceName, a.DurationMin, a.PriceCents,
		a.StartsAt, a.EndsAt, string(a.Status), a.Notes, a.CreatedAt, a.UpdatedAt)
	if _, ok := violation(err, exclusionViolationCode); ok {
		return booking.ErrSlotTaken
	}
	if _, ok := violation(err, foreignKeyViolationCode); ok {
		return booking.ErrUnknownReference
	}
	return err
}

func (s *Appointments) Get(ctx context.Context, id string) (booking.Appointment, error) {
	a, err := scanAppointment(s.pool.QueryRow(ctx, apptSelect+` WHERE a.id = $1`, id))
	if isNoRows(err) {
		return booking.Appointment{}, booking.ErrNotFound
	}
	return a, err
}

func (s *Appointments) List(ctx context.Context, f booking.Filter) ([]booking.Appointment, int, error) {
	var conds []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.Status != "" {
		add("a.status = $%d", string(f.Status))
	}
	if f.CustomerID != "" {
		add("a.customer_id = $%d", f.CustomerID)
	}
	if !f.From.IsZero() {
		add("a.starts_at >= $%d", f.From)
	}
	if !f.To.IsZero() {
		add("a.starts_at < $%d", f.To)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM appointments a`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(apptSelect+where+` ORDER BY a.starts_at, a.id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []booking.Appointment{}
	for rows.Next() {
		a, err := scanAppointment(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

func (s *Appointments) Upcoming(ctx context.Context, now time.Time, limit int) ([]booking.Appointment, error) {
	rows, err := s.pool.Query(ctx, apptSelect+` WHERE a.status = 'scheduled' AND a.starts_at >= $1 ORDER BY a.starts_at, a.id LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []booking.Appointment{}
	for rows.Next() {
		a, err := scanAppointment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Appointments) Transition(ctx context.Context, id string, from, to booking.Status, at time.Time) (booking.Appointment, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE appointments SET status = $3, updated_at = $4 WHERE id = $1 AND status = $2`,
		id, string(from), string(to), at)
	if err != nil {
		return booking.Appointment{}, err
	}
	if tag.RowsAffected() == 0 {
		// Either it does not exist or someone else moved it first.
		if _, err := s.Get(ctx, id); err != nil {
			return booking.Appointment{}, err
		}
		return booking.Appointment{}, booking.ErrInvalidTransition
	}
	return s.Get(ctx, id)
}
