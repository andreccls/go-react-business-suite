package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andreccls/go-react-business-suite/backend/internal/auth"
	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
	"github.com/andreccls/go-react-business-suite/backend/internal/postgres"
	"github.com/andreccls/go-react-business-suite/backend/internal/repotest"
	"github.com/andreccls/go-react-business-suite/backend/internal/testdb"
)

// freshPool returns a pool on an empty, migrated, isolated schema.
func freshPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := postgres.Open(ctx, testdb.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func freshStores(t *testing.T) repotest.Stores {
	pool := freshPool(t)
	return repotest.Stores{
		Catalog: postgres.NewCatalog(pool), Customers: postgres.NewCustomers(pool),
		Appointments: postgres.NewAppointments(pool), Dashboard: postgres.NewDashboard(pool),
	}
}

func TestCatalogContract(t *testing.T)      { repotest.Catalog(t, freshStores) }
func TestCustomersContract(t *testing.T)    { repotest.Customers(t, freshStores) }
func TestAppointmentsContract(t *testing.T) { repotest.Appointments(t, freshStores) }
func TestDashboardContract(t *testing.T)    { repotest.Dashboard(t, freshStores) }

func TestUsersContract(t *testing.T) {
	repotest.AuthStore(t, func(t *testing.T) auth.Store { return postgres.NewUsers(freshPool(t)) })
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := freshPool(t)
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&n); err != nil || n != 2 {
		t.Errorf("schema_migrations rows = %d, err = %v", n, err)
	}
}

func TestMigrateConcurrentStart(t *testing.T) {
	ctx := context.Background()
	pool, err := postgres.Open(ctx, testdb.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() { errs <- postgres.Migrate(ctx, pool) }()
	}
	for i := 0; i < 4; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent migrate: %v", err)
		}
	}
}

func TestOpenFailsFastOnBadURL(t *testing.T) {
	ctx := context.Background()
	if _, err := postgres.Open(ctx, "not a url"); err == nil {
		t.Error("expected parse error")
	}
	if _, err := postgres.Open(ctx, "postgres://u:p@127.0.0.1:1/db?connect_timeout=1"); err == nil {
		t.Error("expected ping error")
	}
}

func TestMigrateReportsBrokenConnection(t *testing.T) {
	ctx := context.Background()
	pool, err := postgres.Open(ctx, testdb.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	if err := postgres.Migrate(ctx, pool); err == nil {
		t.Error("expected error on a closed pool")
	}
}

// With the pool closed every query fails; the repositories must surface that error
// (never swallow it or map it to a domain error such as "not found" or "slot taken").
func TestRepositoriesSurfaceDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	pool := freshPool(t)
	cat, cust, appts := postgres.NewCatalog(pool), postgres.NewCustomers(pool), postgres.NewAppointments(pool)
	dash, users := postgres.NewDashboard(pool), postgres.NewUsers(pool)
	pool.Close()

	id := "6f1b1c3e-0b0e-4a53-9a43-3f0f0c1d2e3f"
	now := time.Now()
	checks := map[string]error{
		"Catalog.Create":      cat.Create(ctx, catalog.Item{ID: id}),
		"Catalog.Update":      cat.Update(ctx, catalog.Item{ID: id}),
		"Catalog.Delete":      cat.Delete(ctx, id),
		"Customers.Create":    cust.Create(ctx, customer.Customer{ID: id}),
		"Customers.Update":    cust.Update(ctx, customer.Customer{ID: id}),
		"Customers.Delete":    cust.Delete(ctx, id),
		"Appointments.Create": appts.Create(ctx, booking.Appointment{ID: id}),
		"CreateUser":          users.CreateUser(ctx, auth.User{ID: id}),
		"SaveRefreshToken":    users.SaveRefreshToken(ctx, auth.RefreshToken{UserID: id}),
		"RevokeRefreshToken":  users.RevokeRefreshToken(ctx, "x"),
	}
	_, checks["Catalog.Get"] = cat.Get(ctx, id)
	_, _, checks["Catalog.List"] = cat.List(ctx, catalog.Filter{Page: 1, PageSize: 1})
	_, checks["Customers.Get"] = cust.Get(ctx, id)
	_, _, checks["Customers.List"] = cust.List(ctx, customer.Filter{Page: 1, PageSize: 1})
	_, checks["Appointments.Get"] = appts.Get(ctx, id)
	_, _, checks["Appointments.List"] = appts.List(ctx, booking.Filter{Page: 1, PageSize: 1})
	_, checks["Appointments.List+filters"] = func() (int, error) {
		_, _, err := appts.List(ctx, booking.Filter{Status: booking.Scheduled, CustomerID: id, From: now, To: now, Page: 1, PageSize: 1})
		return 0, err
	}()
	_, checks["Appointments.Upcoming"] = appts.Upcoming(ctx, now, 1)
	_, checks["Appointments.Transition"] = appts.Transition(ctx, id, booking.Scheduled, booking.Completed, now)
	_, checks["Dashboard.StatusTotals"] = dash.StatusTotals(ctx, now, now)
	_, checks["Dashboard.NewCustomers"] = dash.NewCustomers(ctx, now, now)
	_, checks["Dashboard.Days"] = dash.Days(ctx, now, now, time.UTC)
	_, checks["Dashboard.TopServices"] = dash.TopServices(ctx, now, now, 1)
	_, checks["UserByEmail"] = users.UserByEmail(ctx, "a@example.com")
	_, checks["UserByID"] = users.UserByID(ctx, id)
	_, checks["ConsumeRefreshToken"] = users.ConsumeRefreshToken(ctx, "x", now)
	for name, err := range checks {
		if err == nil {
			t.Errorf("%s: expected an error from a closed pool", name)
		}
		for _, domain := range []error{catalog.ErrNotFound, catalog.ErrInUse, customer.ErrNotFound, customer.ErrInUse, customer.ErrEmailTaken,
			booking.ErrNotFound, booking.ErrSlotTaken, booking.ErrUnknownReference, booking.ErrInvalidTransition,
			auth.ErrNotFound, auth.ErrInvalidToken, auth.ErrEmailTaken} {
			if errors.Is(err, domain) {
				t.Errorf("%s: a database failure was reported as a domain error: %v", name, err)
			}
		}
	}
}

// The overlap rule must live in the schema, not only in Go: a raw INSERT that bypasses
// the repository is refused by the exclusion constraint too.
func TestExclusionConstraintGuardsRawInserts(t *testing.T) {
	ctx := context.Background()
	pool := freshPool(t)
	_, err := pool.Exec(ctx, `
		INSERT INTO services (id, name, duration_min, price_cents, created_at, updated_at)
		  VALUES ('00000000-0000-0000-0000-000000000001', 's', 60, 100, now(), now());
		INSERT INTO customers (id, name, email, created_at, updated_at)
		  VALUES ('00000000-0000-0000-0000-000000000002', 'c', 'c@example.com', now(), now());
		INSERT INTO appointments (id, customer_id, service_id, service_name, duration_min, price_cents, starts_at, ends_at, status, created_at, updated_at)
		  VALUES ('00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 's', 60, 100,
		          '2030-01-01T10:00:00Z', '2030-01-01T11:00:00Z', 'scheduled', now(), now())`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO appointments (id, customer_id, service_id, service_name, duration_min, price_cents, starts_at, ends_at, status, created_at, updated_at)
		VALUES ('00000000-0000-0000-0000-0000000000a2', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 's', 60, 100,
		        '2030-01-01T10:30:00Z', '2030-01-01T11:30:00Z', 'scheduled', now(), now())`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "appointments_no_overlap" {
		t.Errorf("err = %v, want the appointments_no_overlap exclusion violation", err)
	}
}
