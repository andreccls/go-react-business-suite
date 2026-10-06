package dashboard_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
	"github.com/andreccls/go-react-business-suite/backend/internal/dashboard"
	"github.com/andreccls/go-react-business-suite/backend/internal/memstore"
	"github.com/andreccls/go-react-business-suite/backend/internal/validation"
)

var (
	ctx  = context.Background()
	boom = errors.New("boom")
	// now = 2026-03-10 12:00 UTC (09:00 in São Paulo)
	now = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
)

type seeded struct {
	svc      *dashboard.Service
	db       *memstore.DB
	cut, col catalog.Item
	cust     customer.Customer
}

func newSeeded(t *testing.T) *seeded {
	t.Helper()
	sp, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	db := memstore.New()
	s := &seeded{db: db, svc: dashboard.NewService(db.Dashboard(), db.Appointments(), sp, func() time.Time { return now })}
	s.cut = catalog.Item{ID: uuid.NewString(), Name: "Cut", DurationMin: 30, PriceCents: 5000, Active: true}
	s.col = catalog.Item{ID: uuid.NewString(), Name: "Color", DurationMin: 30, PriceCents: 20000, Active: true}
	s.cust = customer.Customer{ID: uuid.NewString(), Name: "Ana", Email: "a@example.com", CreatedAt: time.Date(2026, 3, 6, 15, 0, 0, 0, time.UTC)}
	for _, it := range []catalog.Item{s.cut, s.col} {
		if err := db.Catalog().Create(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Customers().Create(ctx, s.cust); err != nil {
		t.Fatal(err)
	}
	return s
}

// add books an appointment at day/hour (UTC, March 2026) without going through booking.Service.
func (s *seeded) add(t *testing.T, it catalog.Item, day, hour int, st booking.Status) {
	t.Helper()
	start := time.Date(2026, 3, day, hour, 0, 0, 0, time.UTC)
	err := s.db.Appointments().Create(ctx, booking.Appointment{
		ID: uuid.NewString(), CustomerID: s.cust.ID, ServiceID: it.ID, ServiceName: it.Name, StartsAt: start,
		EndsAt: start.Add(30 * time.Minute), DurationMin: 30, PriceCents: it.PriceCents, Status: st,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSummaryKPIs(t *testing.T) {
	s := newSeeded(t)
	// 03-05..03-07 local. Three completed (5000+5000+20000), 1 cancelled, 1 no-show, 1 scheduled.
	s.add(t, s.cut, 5, 13, booking.Completed)
	s.add(t, s.cut, 5, 14, booking.Completed)
	s.add(t, s.col, 6, 13, booking.Completed)
	s.add(t, s.cut, 6, 15, booking.Cancelled)
	s.add(t, s.cut, 7, 13, booking.NoShow)
	s.add(t, s.cut, 7, 15, booking.Scheduled)
	s.add(t, s.cut, 20, 13, booking.Completed) // outside the period

	got, err := s.svc.Summary(ctx, "2026-03-05", "2026-03-07")
	if err != nil {
		t.Fatal(err)
	}
	want := dashboard.Summary{
		Period: dashboard.Period{From: "2026-03-05", To: "2026-03-07", Timezone: "America/Sao_Paulo"},
		AppointmentsTotal: 6, RevenueCents: 30000, AverageTicketCents: 10000,
		CancellationRate: 0.1667, NoShowRate: 0.1667, NewCustomers: 1,
		ByStatus: map[string]int{"scheduled": 1, "completed": 3, "cancelled": 1, "no_show": 1},
	}
	if got.From != want.From || got.To != want.To || got.Timezone != want.Timezone || got.AppointmentsTotal != want.AppointmentsTotal ||
		got.RevenueCents != want.RevenueCents || got.AverageTicketCents != want.AverageTicketCents ||
		got.CancellationRate != want.CancellationRate || got.NoShowRate != want.NoShowRate || got.NewCustomers != want.NewCustomers {
		t.Errorf("summary = %+v\nwant      %+v", got, want)
	}
	for k, v := range want.ByStatus {
		if got.ByStatus[k] != v {
			t.Errorf("by_status[%s] = %d, want %d", k, got.ByStatus[k], v)
		}
	}
}

func TestSummaryRatesTicketRoundingAndEmptyPeriod(t *testing.T) {
	s := newSeeded(t)
	s.add(t, s.cut, 5, 13, booking.Completed)
	s.add(t, s.col, 5, 15, booking.Completed)
	s.add(t, s.cut, 5, 17, booking.Completed)
	s.add(t, s.cut, 5, 19, booking.Cancelled)
	s.add(t, s.cut, 5, 20, booking.Cancelled)
	s.add(t, s.cut, 5, 21, booking.Cancelled)
	g, _ := s.svc.Summary(ctx, "2026-03-05", "2026-03-05")
	if g.CancellationRate != 0.5 || g.NoShowRate != 0 || g.AverageTicketCents != 10000 {
		t.Errorf("rates = %v / %v, ticket = %d", g.CancellationRate, g.NoShowRate, g.AverageTicketCents)
	}

	half := newSeeded(t)
	half.cut.PriceCents = 5001
	half.add(t, half.cut, 5, 13, booking.Completed)
	half.cut.PriceCents = 5000
	half.add(t, half.cut, 5, 15, booking.Completed) // 10001 / 2 = 5000.5
	h, _ := half.svc.Summary(ctx, "2026-03-05", "2026-03-05")
	if h.AverageTicketCents != 5001 {
		t.Errorf("half-cent ticket = %d, want 5001 (round half up)", h.AverageTicketCents)
	}

	empty, err := newSeeded(t).svc.Summary(ctx, "2026-01-01", "2026-01-31")
	if err != nil || empty.AppointmentsTotal != 0 || empty.RevenueCents != 0 || empty.AverageTicketCents != 0 ||
		empty.CancellationRate != 0 || empty.NoShowRate != 0 || len(empty.ByStatus) != 4 {
		t.Errorf("empty period must be all zeros (no NaN, no division by zero): %+v, %v", empty, err)
	}
}

func TestSummaryDefaultsToLast30Days(t *testing.T) {
	got, err := newSeeded(t).svc.Summary(ctx, "", "")
	if err != nil || got.To != "2026-03-10" || got.From != "2026-02-09" {
		t.Errorf("defaults = %q..%q, %v", got.From, got.To, err)
	}
}

func TestPeriodValidation(t *testing.T) {
	s := newSeeded(t)
	if _, err := s.svc.Summary(ctx, "2026-03-05", "2026-03-01"); err == nil {
		t.Error("inverted period must fail")
	}
	if _, err := s.svc.Daily(ctx, "x", ""); err == nil {
		t.Error("bad date must fail")
	}
	if _, err := s.svc.TopServices(ctx, "2025-01-01", "2026-12-31", 0); err == nil {
		t.Error("too long a period must fail")
	}
}

func TestDailyIsZeroFilled(t *testing.T) {
	s := newSeeded(t)
	s.add(t, s.cut, 5, 13, booking.Completed)
	s.add(t, s.col, 5, 14, booking.Scheduled)
	s.add(t, s.cut, 7, 13, booking.Completed)
	series, err := s.svc.Daily(ctx, "2026-03-04", "2026-03-08")
	if err != nil {
		t.Fatal(err)
	}
	if series.From != "2026-03-04" || series.To != "2026-03-08" || series.Timezone != "America/Sao_Paulo" {
		t.Errorf("period = %+v", series.Period)
	}
	got := series.Data
	want := []dashboard.Day{
		{Date: "2026-03-04"},
		{Date: "2026-03-05", Appointments: 2, Completed: 1, RevenueCents: 5000},
		{Date: "2026-03-06"},
		{Date: "2026-03-07", Appointments: 1, Completed: 1, RevenueCents: 5000},
		{Date: "2026-03-08"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("day %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	one, err := s.svc.Daily(ctx, "2026-03-05", "2026-03-05")
	if err != nil || len(one.Data) != 1 || one.Data[0].Appointments != 2 {
		t.Errorf("single day = %+v, %v", one, err)
	}
}

func TestTopServicesAndUpcoming(t *testing.T) {
	s := newSeeded(t)
	s.add(t, s.cut, 5, 13, booking.Completed)
	s.add(t, s.col, 5, 14, booking.Completed)
	got, err := s.svc.TopServices(ctx, "2026-03-05", "2026-03-05", 0)
	if err != nil || len(got.Data) != 2 || got.Data[0].Name != "Color" || got.Data[0].RevenueCents != 20000 || got.Data[1].Name != "Cut" || got.From != "2026-03-05" {
		t.Errorf("top = %+v, %v", got, err)
	}
	if one, _ := s.svc.TopServices(ctx, "2026-03-05", "2026-03-05", 1); len(one.Data) != 1 {
		t.Errorf("limit 1 = %+v", one)
	}
	for _, limit := range []int{-1, 21} {
		_, err := s.svc.TopServices(ctx, "", "", limit)
		var v validation.Errors
		if !errors.As(err, &v) || v[0].Field != "limit" {
			t.Errorf("limit %d: %v", limit, err)
		}
	}
	if none, err := s.svc.TopServices(ctx, "2025-01-01", "2025-01-02", 5); err != nil || none.Data == nil || len(none.Data) != 0 {
		t.Errorf("empty must be [] not null: %v, %v", none, err)
	}

	// Upcoming: scheduled only, from now (03-10 12:00Z) on.
	s.add(t, s.cut, 10, 11, booking.Scheduled) // before now
	s.add(t, s.cut, 11, 13, booking.Scheduled)
	s.add(t, s.cut, 12, 13, booking.Scheduled)
	s.add(t, s.cut, 13, 13, booking.Cancelled)
	up, err := s.svc.UpcomingAppointments(ctx, 0)
	if err != nil || len(up) != 2 || up[0].StartsAt.Day() != 11 || up[0].CustomerName != "Ana" {
		t.Errorf("upcoming = %+v, %v", up, err)
	}
	if one, _ := s.svc.UpcomingAppointments(ctx, 1); len(one) != 1 {
		t.Errorf("limit 1 = %+v", one)
	}
	if _, err := s.svc.UpcomingAppointments(ctx, 21); err == nil {
		t.Error("limit 21 must fail")
	}
	if _, err := s.svc.UpcomingAppointments(ctx, -2); err == nil {
		t.Error("limit -2 must fail")
	}
	if none, err := newSeeded(t).svc.UpcomingAppointments(ctx, 5); err != nil || none == nil || len(none) != 0 {
		t.Errorf("nothing upcoming must be [] not null: %v, %v", none, err)
	}
}

// brokenReader fails one call at a time.
type brokenReader struct {
	dashboard.Reader
	failTotals, failNew, failDays, failTop bool
}

func (b brokenReader) StatusTotals(ctx context.Context, f, t time.Time) (map[booking.Status]dashboard.StatusTotals, error) {
	if b.failTotals {
		return nil, boom
	}
	return b.Reader.StatusTotals(ctx, f, t)
}
func (b brokenReader) NewCustomers(ctx context.Context, f, t time.Time) (int, error) {
	if b.failNew {
		return 0, boom
	}
	return b.Reader.NewCustomers(ctx, f, t)
}
func (b brokenReader) Days(ctx context.Context, f, t time.Time, l *time.Location) ([]dashboard.Day, error) {
	if b.failDays {
		return nil, boom
	}
	return b.Reader.Days(ctx, f, t, l)
}
func (b brokenReader) TopServices(ctx context.Context, f, t time.Time, n int) ([]dashboard.TopService, error) {
	if b.failTop {
		return nil, boom
	}
	return b.Reader.TopServices(ctx, f, t, n)
}

type brokenUpcoming struct{}

func (brokenUpcoming) Upcoming(context.Context, time.Time, int) ([]booking.Appointment, error) {
	return nil, boom
}

func TestStorageErrorsPropagate(t *testing.T) {
	db := memstore.New()
	mk := func(b brokenReader) *dashboard.Service {
		b.Reader = db.Dashboard()
		return dashboard.NewService(b, brokenUpcoming{}, time.UTC, func() time.Time { return now })
	}
	if _, err := mk(brokenReader{failTotals: true}).Summary(ctx, "", ""); !errors.Is(err, boom) {
		t.Errorf("totals: %v", err)
	}
	if _, err := mk(brokenReader{failNew: true}).Summary(ctx, "", ""); !errors.Is(err, boom) {
		t.Errorf("new customers: %v", err)
	}
	if _, err := mk(brokenReader{failDays: true}).Daily(ctx, "", ""); !errors.Is(err, boom) {
		t.Errorf("days: %v", err)
	}
	if _, err := mk(brokenReader{failTop: true}).TopServices(ctx, "", "", 3); !errors.Is(err, boom) {
		t.Errorf("top: %v", err)
	}
	if _, err := mk(brokenReader{}).UpcomingAppointments(ctx, 3); !errors.Is(err, boom) {
		t.Errorf("upcoming: %v", err)
	}
}

func TestNewServiceDefaultsToWallClock(t *testing.T) {
	db := memstore.New()
	got, err := dashboard.NewService(db.Dashboard(), db.Appointments(), time.UTC, nil).Summary(ctx, "", "")
	if err != nil || got.To != time.Now().UTC().Format(time.DateOnly) {
		t.Errorf("to = %q, %v", got.To, err)
	}
}
