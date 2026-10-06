package booking_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
	"github.com/andreccls/go-react-business-suite/backend/internal/memstore"
	"github.com/andreccls/go-react-business-suite/backend/internal/validation"
)

var (
	ctx  = context.Background()
	boom = errors.New("boom")
	// "now" is Monday 2026-03-02 12:00 UTC (09:00 in São Paulo).
	t0 = time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
)

type fixture struct {
	svc      *booking.Service
	catalog  *catalog.Service
	custs    *customer.Service
	repo     booking.Repository
	now      time.Time
	cust     customer.Customer
	cut      catalog.Item // 30 min, R$50
	inactive catalog.Item
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	sp, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	db := memstore.New()
	f := &fixture{now: t0, repo: db.Appointments()}
	clock := func() time.Time { return f.now }
	f.catalog = catalog.NewService(db.Catalog(), clock)
	f.custs = customer.NewService(db.Customers(), clock)
	f.svc = booking.NewService(f.repo, f.catalog, f.custs, sp, clock)
	if f.cust, err = f.custs.Create(ctx, customer.Input{Name: "Ana Souza", Email: "ana@example.com"}); err != nil {
		t.Fatal(err)
	}
	if f.cut, err = f.catalog.Create(ctx, catalog.Input{Name: "Cut", DurationMin: 30, PriceCents: 5000}); err != nil {
		t.Fatal(err)
	}
	off := false
	if f.inactive, err = f.catalog.Create(ctx, catalog.Input{Name: "Retired", DurationMin: 30, PriceCents: 1, Active: &off}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) in(start time.Time) booking.Input {
	return booking.Input{CustomerID: f.cust.ID, ServiceID: f.cut.ID, StartsAt: start}
}

func fields(err error) string {
	var v validation.Errors
	if !errors.As(err, &v) {
		return ""
	}
	var out []string
	for _, e := range v {
		out = append(out, e.Field)
	}
	return strings.Join(out, ",")
}

func TestStatusStateMachine(t *testing.T) {
	all := []booking.Status{booking.Scheduled, booking.Completed, booking.Cancelled, booking.NoShow}
	for _, from := range all {
		for _, to := range all {
			want := from == booking.Scheduled && to != booking.Scheduled
			if got := from.CanTransitionTo(to); got != want {
				t.Errorf("%s -> %s = %v, want %v", from, to, got, want)
			}
		}
		if !from.Valid() {
			t.Errorf("%s must be valid", from)
		}
		if want := from == booking.Scheduled || from == booking.Completed; from.OccupiesAgenda() != want {
			t.Errorf("%s OccupiesAgenda = %v", from, from.OccupiesAgenda())
		}
	}
	if booking.Status("paid").Valid() || booking.Status("").Valid() {
		t.Error("unknown statuses must be invalid")
	}
}

func TestCreateSnapshotsServiceAndDerivesEnd(t *testing.T) {
	f := newFixture(t)
	start := time.Date(2026, 3, 3, 14, 0, 0, 0, time.FixedZone("BRT", -3*3600)) // 17:00Z
	in := f.in(start)
	in.Notes = "  first visit "
	a, err := f.svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(a.ID); err != nil || a.Status != booking.Scheduled || a.Notes != "first visit" ||
		a.CustomerID != f.cust.ID || a.CustomerName != "Ana Souza" || a.ServiceID != f.cut.ID {
		t.Errorf("unexpected appointment: %+v", a)
	}
	if !a.StartsAt.Equal(start) || a.StartsAt.Location() != time.UTC {
		t.Errorf("start = %v (must be the same instant, in UTC)", a.StartsAt)
	}
	if want := a.StartsAt.Add(30 * time.Minute); !a.EndsAt.Equal(want) {
		t.Errorf("end = %v, want %v (start + service duration)", a.EndsAt, want)
	}
	if a.ServiceName != "Cut" || a.DurationMin != 30 || a.PriceCents != 5000 {
		t.Errorf("snapshot = %q %d min %d cents", a.ServiceName, a.DurationMin, a.PriceCents)
	}

	// The snapshot is frozen: editing the service later does not rewrite the appointment.
	name, dur, price := "Cut Deluxe", 90, 99900
	if _, err := f.catalog.Update(ctx, f.cut.ID, catalog.Patch{Name: &name, DurationMin: &dur, PriceCents: &price}); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Get(ctx, a.ID)
	if err != nil || got.ServiceName != "Cut" || got.DurationMin != 30 || got.PriceCents != 5000 || !got.EndsAt.Equal(a.EndsAt) {
		t.Errorf("snapshot changed after the service was edited: %+v (%v)", got, err)
	}
	// A new booking takes the new values.
	b, err := f.svc.Create(ctx, f.in(start.Add(24*time.Hour)))
	if err != nil || b.ServiceName != "Cut Deluxe" || b.DurationMin != 90 || b.PriceCents != 99900 || !b.EndsAt.Equal(b.StartsAt.Add(90*time.Minute)) {
		t.Errorf("new booking: %+v (%v)", b, err)
	}
}

func TestCreateRules(t *testing.T) {
	f := newFixture(t)
	future := t0.Add(time.Hour)
	tests := []struct {
		name    string
		mutate  func(in *booking.Input)
		wantErr error
		fields  string
	}{
		{"valid", func(*booking.Input) {}, nil, ""},
		{"in the past", func(in *booking.Input) { in.StartsAt = t0.Add(-time.Minute) }, nil, "starts_at"},
		{"right now is not the future", func(in *booking.Input) { in.StartsAt = t0 }, nil, "starts_at"},
		{"sub-microsecond is truncated before comparing", func(in *booking.Input) { in.StartsAt = t0.Add(time.Nanosecond) }, nil, "starts_at"},
		{"missing start", func(in *booking.Input) { in.StartsAt = time.Time{} }, nil, "starts_at"},
		{"malformed customer id", func(in *booking.Input) { in.CustomerID = "x" }, nil, "customer_id"},
		{"malformed service id", func(in *booking.Input) { in.ServiceID = "" }, nil, "service_id"},
		{"notes too long", func(in *booking.Input) { in.Notes = strings.Repeat("n", 501) }, nil, "notes"},
		{"everything wrong", func(in *booking.Input) { *in = booking.Input{Notes: strings.Repeat("n", 501)} }, nil, "notes,starts_at,customer_id,service_id"},
		{"unknown customer", func(in *booking.Input) { in.CustomerID = uuid.NewString() }, nil, "customer_id"},
		{"unknown service", func(in *booking.Input) { in.ServiceID = uuid.NewString() }, nil, "service_id"},
		{"inactive service", func(in *booking.Input) { in.ServiceID = f.inactive.ID }, booking.ErrServiceInactive, ""},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := f.in(future.Add(time.Duration(i) * time.Hour)) // distinct slots: only the rule under test can fail
			tt.mutate(&in)
			_, err := f.svc.Create(ctx, in)
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("err = %v, want %v", err, tt.wantErr)
				}
			case fields(err) != tt.fields:
				t.Errorf("fields = %q, want %q (%v)", fields(err), tt.fields, err)
			}
		})
	}
}

func TestCreateSlotConflict(t *testing.T) {
	f := newFixture(t)
	start := t0.Add(2 * time.Hour)
	if _, err := f.svc.Create(ctx, f.in(start)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, f.in(start.Add(15*time.Minute))); !errors.Is(err, booking.ErrSlotTaken) {
		t.Errorf("overlap: %v", err)
	}
	if _, err := f.svc.Create(ctx, f.in(start.Add(30*time.Minute))); err != nil {
		t.Errorf("back-to-back must be allowed: %v", err)
	}
}

// stubs make one collaborator fail.
type (
	badCustomers struct{}
	badCatalog   struct{}
)

func (badCustomers) Get(context.Context, string) (customer.Customer, error) {
	return customer.Customer{}, boom
}
func (badCatalog) Get(context.Context, string) (catalog.Item, error) { return catalog.Item{}, boom }

func TestCreatePropagatesCollaboratorErrors(t *testing.T) {
	f := newFixture(t)
	in := f.in(t0.Add(time.Hour))
	if _, err := booking.NewService(f.repo, f.catalog, badCustomers{}, time.UTC, f.svcNow()).Create(ctx, in); !errors.Is(err, boom) {
		t.Errorf("customers: %v", err)
	}
	if _, err := booking.NewService(f.repo, badCatalog{}, f.custs, time.UTC, f.svcNow()).Create(ctx, in); !errors.Is(err, boom) {
		t.Errorf("catalog: %v", err)
	}
}

func (f *fixture) svcNow() func() time.Time { return func() time.Time { return f.now } }

func TestGetWithMalformedID(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Get(ctx, "nope"); !errors.Is(err, booking.ErrNotFound) {
		t.Errorf("malformed: %v", err)
	}
	if _, err := f.svc.Get(ctx, uuid.NewString()); !errors.Is(err, booking.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

func TestSetStatus(t *testing.T) {
	f := newFixture(t)
	book := func(start time.Time) booking.Appointment {
		t.Helper()
		a, err := f.svc.Create(ctx, f.in(start))
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	start := t0.Add(3 * time.Hour) // 15:00Z

	// Cancelling is allowed before the start; the slot is released.
	a := book(start)
	got, err := f.svc.SetStatus(ctx, a.ID, booking.Cancelled)
	if err != nil || got.Status != booking.Cancelled || !got.UpdatedAt.Equal(t0) {
		t.Fatalf("cancel: %+v, %v", got, err)
	}
	if _, err := f.svc.Create(ctx, f.in(start)); err != nil {
		t.Errorf("cancelled slot must be bookable again: %v", err)
	}
	// Terminal states do not move.
	for _, to := range []booking.Status{booking.Scheduled, booking.Completed, booking.Cancelled, booking.NoShow} {
		if _, err := f.svc.SetStatus(ctx, a.ID, to); !errors.Is(err, booking.ErrInvalidTransition) {
			t.Errorf("cancelled -> %s: %v", to, err)
		}
	}

	// Completing / no-show need the appointment to have started.
	b := book(t0.Add(5 * time.Hour))
	for _, to := range []booking.Status{booking.Completed, booking.NoShow} {
		if _, err := f.svc.SetStatus(ctx, b.ID, to); !errors.Is(err, booking.ErrNotStarted) {
			t.Errorf("scheduled (future) -> %s: %v", to, err)
		}
	}
	f.now = b.StartsAt // exactly at the start: allowed
	done, err := f.svc.SetStatus(ctx, b.ID, booking.Completed)
	if err != nil || done.Status != booking.Completed || !done.UpdatedAt.Equal(b.StartsAt) {
		t.Errorf("complete: %+v, %v", done, err)
	}
	if _, err := f.svc.SetStatus(ctx, b.ID, booking.NoShow); !errors.Is(err, booking.ErrInvalidTransition) {
		t.Errorf("completed -> no_show: %v", err)
	}

	// no_show frees the slot.
	f.now = t0
	c := book(t0.Add(7 * time.Hour))
	f.now = c.StartsAt.Add(time.Minute)
	if ns, err := f.svc.SetStatus(ctx, c.ID, booking.NoShow); err != nil || ns.Status != booking.NoShow {
		t.Errorf("no_show: %+v, %v", ns, err)
	}

	// Scheduled -> scheduled and unknown values.
	f.now = t0
	d := book(t0.Add(9 * time.Hour))
	if _, err := f.svc.SetStatus(ctx, d.ID, booking.Scheduled); !errors.Is(err, booking.ErrInvalidTransition) {
		t.Errorf("scheduled -> scheduled: %v", err)
	}
	if _, err := f.svc.SetStatus(ctx, d.ID, "paid"); fields(err) != "status" {
		t.Errorf("unknown status: %v", err)
	}
	if _, err := f.svc.SetStatus(ctx, "nope", booking.Cancelled); !errors.Is(err, booking.ErrNotFound) {
		t.Errorf("malformed id: %v", err)
	}
	if _, err := f.svc.SetStatus(ctx, uuid.NewString(), booking.Cancelled); !errors.Is(err, booking.ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}

func TestList(t *testing.T) {
	f := newFixture(t)
	// 2026-03-03 10:00 BRT = 13:00Z, 2026-03-04 22:30 BRT = 03-05 01:30Z.
	day3, _ := f.svc.Create(ctx, f.in(time.Date(2026, 3, 3, 13, 0, 0, 0, time.UTC)))
	day4, _ := f.svc.Create(ctx, f.in(time.Date(2026, 3, 5, 1, 30, 0, 0, time.UTC)))
	_, _ = f.svc.SetStatus(ctx, day4.ID, booking.Cancelled)

	ids := func(q booking.Query) string {
		t.Helper()
		q.Page, q.PageSize = 1, 50
		items, _, err := f.svc.List(ctx, q)
		if err != nil {
			t.Fatalf("%+v: %v", q, err)
		}
		var out []string
		for _, a := range items {
			switch a.ID {
			case day3.ID:
				out = append(out, "day3")
			case day4.ID:
				out = append(out, "day4")
			}
		}
		return strings.Join(out, ",")
	}
	for name, tt := range map[string]struct {
		q    booking.Query
		want string
	}{
		"all":                       {booking.Query{}, "day3,day4"},
		"status":                    {booking.Query{Status: booking.Cancelled}, "day4"},
		"customer":                  {booking.Query{CustomerID: f.cust.ID}, "day3,day4"},
		"day in the business zone":  {booking.Query{From: "2026-03-04", To: "2026-03-04"}, "day4"}, // 01:30Z is still 03-04 locally
		"from only":                 {booking.Query{From: "2026-03-04"}, "day4"},
		"to only":                   {booking.Query{To: "2026-03-03"}, "day3"},
		"range with nothing inside": {booking.Query{From: "2026-04-01", To: "2026-04-02"}, ""},
	} {
		if got := ids(tt.q); got != tt.want {
			t.Errorf("%s: %q, want %q", name, got, tt.want)
		}
	}

	for name, q := range map[string]booking.Query{
		"page 0":       {Page: 0, PageSize: 10},
		"size 0":       {Page: 1, PageSize: 0},
		"size too big": {Page: 1, PageSize: 101},
		"bad status":   {Page: 1, PageSize: 10, Status: "paid"},
		"bad customer": {Page: 1, PageSize: 10, CustomerID: "x"},
		"bad dates":    {Page: 1, PageSize: 10, From: "yesterday", To: "2026-03-01"},
		"inverted":     {Page: 1, PageSize: 10, From: "2026-03-05", To: "2026-03-01"},
	} {
		if _, _, err := f.svc.List(ctx, q); fields(err) == "" {
			t.Errorf("%s: expected a validation error, got %v", name, err)
		}
	}
	if _, _, err := f.svc.List(ctx, booking.Query{Page: 1, PageSize: 10, Status: booking.NoShow}); err != nil {
		t.Errorf("valid status: %v", err)
	}
}

func TestNewServiceDefaultsToWallClock(t *testing.T) {
	db := memstore.New()
	svc := booking.NewService(db.Appointments(), catalog.NewService(db.Catalog(), nil), customer.NewService(db.Customers(), nil), time.UTC, nil)
	_, err := svc.Create(ctx, booking.Input{CustomerID: uuid.NewString(), ServiceID: uuid.NewString(), StartsAt: time.Now().Add(-time.Hour)})
	if fields(err) != "starts_at" {
		t.Errorf("a start one hour ago must be refused by the real clock: %v", err)
	}
}
