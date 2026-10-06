// Package dashboard computes the business indicators. The Repository only does the
// aggregation that must happen next to the data (GROUP BY); every business definition
// (revenue, rates, ticket, zero-filled day series) lives here, in plain testable Go.
package dashboard

import (
	"context"
	"math"
	"time"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/period"
	"github.com/andreccls/go-react-business-suite/backend/internal/validation"
)

// StatusTotals is the number of appointments and the sum of their frozen prices.
type StatusTotals struct {
	Count        int
	RevenueCents int
}

// Reader is the aggregation storage. Ranges are [from, to) instants.
type Reader interface {
	// StatusTotals groups the appointments STARTING in the range by status.
	StatusTotals(ctx context.Context, from, to time.Time) (map[booking.Status]StatusTotals, error)
	// NewCustomers counts customers created in the range.
	NewCustomers(ctx context.Context, from, to time.Time) (int, error)
	// Days returns only the days with appointments, ordered by date; day boundaries
	// are in loc.
	Days(ctx context.Context, from, to time.Time, loc *time.Location) ([]Day, error)
	// TopServices orders by realized revenue desc, completed desc, appointments desc, name.
	TopServices(ctx context.Context, from, to time.Time, limit int) ([]TopService, error)
}

// Upcoming lists the next scheduled appointments (the booking repository does it).
type Upcoming interface {
	Upcoming(ctx context.Context, now time.Time, limit int) ([]booking.Appointment, error)
}

// Defaults and bounds.
const (
	DefaultDays  = 30 // period used when from/to are omitted
	DefaultLimit = 5
	MaxLimit     = 20
)

// Period echoes the (inclusive) dates a result refers to, after the defaults were applied.
type Period struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Timezone string `json:"timezone"`
}

// DailySeries is the day-by-day series of a period.
type DailySeries struct {
	Period
	Data []Day `json:"data"`
}

// Ranking is the top-services ranking of a period.
type Ranking struct {
	Period
	Data []TopService `json:"data"`
}

// Summary holds the KPIs of a period. Rates are ratios in [0, 1] (4 decimals) over
// ALL appointments that start in the period.
type Summary struct {
	Period
	AppointmentsTotal  int            `json:"appointments_total"`
	ByStatus           map[string]int `json:"by_status"`
	RevenueCents       int            `json:"revenue_cents"`
	AverageTicketCents int            `json:"average_ticket_cents"`
	CancellationRate   float64        `json:"cancellation_rate"`
	NoShowRate         float64        `json:"no_show_rate"`
	NewCustomers       int            `json:"new_customers"`
}

// Day is one point of the daily series.
type Day struct {
	Date         string `json:"date"`
	Appointments int    `json:"appointments"`
	Completed    int    `json:"completed"`
	RevenueCents int    `json:"revenue_cents"`
}

// TopService is one entry of the ranking.
type TopService struct {
	ServiceID    string `json:"service_id"`
	Name         string `json:"name"`
	Appointments int    `json:"appointments"`
	Completed    int    `json:"completed"`
	RevenueCents int    `json:"revenue_cents"`
}

// Service holds the dashboard use cases.
type Service struct {
	reader   Reader
	upcoming Upcoming
	loc      *time.Location
	now      func() time.Time
}

// NewService builds a Service. loc is the business time zone; now is injectable.
func NewService(r Reader, u Upcoming, loc *time.Location, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{reader: r, upcoming: u, loc: loc, now: now}
}

func (s *Service) parse(from, to string) (period.Range, Period, error) {
	r, err := period.Parse(from, to, s.loc, s.now(), DefaultDays)
	return r, Period{From: r.FromDay, To: r.ToDay, Timezone: s.loc.String()}, err
}

// Summary returns the KPIs of [from, to] (dates; defaults to the last 30 days).
func (s *Service) Summary(ctx context.Context, from, to string) (Summary, error) {
	r, p, err := s.parse(from, to)
	if err != nil {
		return Summary{}, err
	}
	totals, err := s.reader.StatusTotals(ctx, r.From, r.To)
	if err != nil {
		return Summary{}, err
	}
	newCust, err := s.reader.NewCustomers(ctx, r.From, r.To)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{Period: p, NewCustomers: newCust, ByStatus: map[string]int{}}
	for _, st := range []booking.Status{booking.Scheduled, booking.Completed, booking.Cancelled, booking.NoShow} {
		sum.ByStatus[string(st)] = totals[st].Count
		sum.AppointmentsTotal += totals[st].Count
	}
	done := totals[booking.Completed]
	sum.RevenueCents = done.RevenueCents
	if done.Count > 0 {
		sum.AverageTicketCents = int(math.Round(float64(done.RevenueCents) / float64(done.Count)))
	}
	if sum.AppointmentsTotal > 0 {
		sum.CancellationRate = ratio(totals[booking.Cancelled].Count, sum.AppointmentsTotal)
		sum.NoShowRate = ratio(totals[booking.NoShow].Count, sum.AppointmentsTotal)
	}
	return sum, nil
}

func ratio(n, d int) float64 { return math.Round(float64(n)/float64(d)*10000) / 10000 }

// Daily returns one point per calendar day of the period (zero-filled), so a chart
// never has holes.
func (s *Service) Daily(ctx context.Context, from, to string) (DailySeries, error) {
	r, p, err := s.parse(from, to)
	if err != nil {
		return DailySeries{}, err
	}
	rows, err := s.reader.Days(ctx, r.From, r.To, s.loc)
	if err != nil {
		return DailySeries{}, err
	}
	byDate := make(map[string]Day, len(rows))
	for _, row := range rows {
		byDate[row.Date] = row
	}
	last, _ := time.ParseInLocation(time.DateOnly, r.ToDay, time.UTC)
	days := []Day{}
	for d, _ := time.ParseInLocation(time.DateOnly, r.FromDay, time.UTC); !d.After(last); d = d.AddDate(0, 0, 1) {
		key := d.Format(time.DateOnly)
		row := byDate[key]
		row.Date = key
		days = append(days, row)
	}
	return DailySeries{Period: p, Data: days}, nil
}

// TopServices ranks the services of the period (limit 1..20, default 5).
func (s *Service) TopServices(ctx context.Context, from, to string, limit int) (Ranking, error) {
	r, p, err := s.parse(from, to)
	if err != nil {
		return Ranking{}, err
	}
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		var v validation.Errors
		v.Add("limit", "must be between 1 and 20")
		return Ranking{}, v
	}
	// The Reader contract: never nil, so the JSON is [] and not null.
	rows, err := s.reader.TopServices(ctx, r.From, r.To, limit)
	return Ranking{Period: p, Data: rows}, err
}

// UpcomingAppointments returns the next scheduled appointments (limit 1..20, default 5).
func (s *Service) UpcomingAppointments(ctx context.Context, limit int) ([]booking.Appointment, error) {
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		var v validation.Errors
		v.Add("limit", "must be between 1 and 20")
		return nil, v
	}
	return s.upcoming.Upcoming(ctx, s.now().UTC(), limit)
}
