package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/dashboard"
)

// Dashboard is the PostgreSQL dashboard.Reader. Every query is one range scan on
// appointments.starts_at (index appointments_starts_at_idx) plus a GROUP BY: the work
// grows with the size of the PERIOD, not of the table (see ADR 0004 for EXPLAIN output).
type Dashboard struct{ pool *pgxpool.Pool }

// NewDashboard returns a reader on pool.
func NewDashboard(pool *pgxpool.Pool) *Dashboard { return &Dashboard{pool} }

func (s *Dashboard) StatusTotals(ctx context.Context, from, to time.Time) (map[booking.Status]dashboard.StatusTotals, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT status, count(*), COALESCE(sum(price_cents), 0)
		 FROM appointments WHERE starts_at >= $1 AND starts_at < $2 GROUP BY status`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[booking.Status]dashboard.StatusTotals{}
	for rows.Next() {
		var st string
		var t dashboard.StatusTotals
		if err := rows.Scan(&st, &t.Count, &t.RevenueCents); err != nil {
			return nil, err
		}
		out[booking.Status(st)] = t
	}
	return out, rows.Err()
}

func (s *Dashboard) NewCustomers(ctx context.Context, from, to time.Time) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM customers WHERE created_at >= $1 AND created_at < $2`, from, to).Scan(&n)
	return n, err
}

func (s *Dashboard) Days(ctx context.Context, from, to time.Time, loc *time.Location) ([]dashboard.Day, error) {
	// The local day of an instant: (starts_at AT TIME ZONE zone)::date.
	rows, err := s.pool.Query(ctx,
		`SELECT to_char((starts_at AT TIME ZONE $3::text)::date, 'YYYY-MM-DD') AS day,
		        count(*),
		        count(*) FILTER (WHERE status = 'completed'),
		        COALESCE(sum(price_cents) FILTER (WHERE status = 'completed'), 0)
		 FROM appointments WHERE starts_at >= $1 AND starts_at < $2
		 GROUP BY day ORDER BY day`, from, to, loc.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []dashboard.Day{}
	for rows.Next() {
		var d dashboard.Day
		if err := rows.Scan(&d.Date, &d.Appointments, &d.Completed, &d.RevenueCents); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Dashboard) TopServices(ctx context.Context, from, to time.Time, limit int) ([]dashboard.TopService, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.service_id, s.name, count(*),
		        count(*) FILTER (WHERE a.status = 'completed'),
		        COALESCE(sum(a.price_cents) FILTER (WHERE a.status = 'completed'), 0) AS revenue
		 FROM appointments a JOIN services s ON s.id = a.service_id
		 WHERE a.starts_at >= $1 AND a.starts_at < $2
		 GROUP BY a.service_id, s.name
		 ORDER BY revenue DESC, 4 DESC, 3 DESC, s.name, a.service_id
		 LIMIT $3`, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []dashboard.TopService{}
	for rows.Next() {
		var t dashboard.TopService
		if err := rows.Scan(&t.ServiceID, &t.Name, &t.Appointments, &t.Completed, &t.RevenueCents); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
