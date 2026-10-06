package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andreccls/go-react-business-suite/backend/internal/auth"
	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
	"github.com/andreccls/go-react-business-suite/backend/internal/httpapi"
)

// failingBookings answers every call with err.
type failingBookings struct{ err error }

func (f failingBookings) Create(context.Context, booking.Input) (booking.Appointment, error) {
	return booking.Appointment{}, f.err
}
func (f failingBookings) Get(context.Context, string) (booking.Appointment, error) {
	return booking.Appointment{}, f.err
}
func (f failingBookings) List(context.Context, booking.Query) ([]booking.Appointment, int, error) {
	return nil, 0, f.err
}
func (f failingBookings) SetStatus(context.Context, string, booking.Status) (booking.Appointment, error) {
	return booking.Appointment{}, f.err
}

// TestEveryDomainErrorHasItsHTTPMapping is the table of the error -> status/code
// translation (the single `fail` function), one row per sentinel the domain returns.
func TestEveryDomainErrorHasItsHTTPMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{booking.ErrNotFound, 404, "appointment_not_found"},
		{catalog.ErrNotFound, 404, "service_not_found"},
		{customer.ErrNotFound, 404, "customer_not_found"},
		{booking.ErrSlotTaken, 409, "slot_unavailable"},
		{booking.ErrInvalidTransition, 409, "invalid_transition"},
		{booking.ErrNotStarted, 409, "not_started"},
		{booking.ErrUnknownReference, 409, "unknown_reference"},
		{booking.ErrServiceInactive, 422, "service_inactive"},
		{customer.ErrEmailTaken, 409, "email_taken"},
		{auth.ErrEmailTaken, 409, "email_taken"},
		{customer.ErrInUse, 409, "customer_in_use"},
		{catalog.ErrInUse, 409, "service_in_use"},
		{auth.ErrInvalidCredentials, 401, "invalid_credentials"},
		{auth.ErrInvalidToken, 401, "invalid_token"},
		{context.DeadlineExceeded, 503, "timeout"},
		{errors.New("anything else"), 500, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			e := newEnv(t, func(o *options) {
				o.mutate = func(d *httpapi.Deps) { d.Bookings = failingBookings{tt.err} }
			})
			// Check the translation directly (the spec check below it is relaxed: this
			// stub can produce combinations a real endpoint never would).
			req := httptest.NewRequest("GET", "/v1/appointments/"+unknownID, nil)
			req.Header.Set("Authorization", "Bearer "+e.staffTk)
			rec := httptest.NewRecorder()
			e.h.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.status, rec.Body)
			}
			if got := (resp{status: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}).json(t)["code"]; got != tt.code {
				t.Errorf("code = %v, want %s", got, tt.code)
			}
		})
	}
}

// brokenAuth fails the calls the other tests never make fail.
type brokenAuth struct {
	httpapi.AuthService
	err error
}

func (b brokenAuth) Logout(context.Context, string) error { return b.err }
func (b brokenAuth) Me(context.Context, string) (auth.User, error) {
	return auth.User{}, b.err
}

// nilLists returns nil slices, as a careless implementation might: the JSON must still be [].
type nilCustomers struct {
	httpapi.CustomerService
	err error
}

func (n nilCustomers) List(context.Context, customer.Filter) ([]customer.Customer, int, error) {
	return nil, 0, n.err
}

func TestHandlerErrorPaths(t *testing.T) {
	boom := errors.New("boom")
	e := newEnv(t, func(o *options) {
		o.mutate = func(d *httpapi.Deps) {
			real := d.Auth
			d.Auth = authSwitch{AuthService: real, broken: brokenAuth{AuthService: real, err: boom}}
			d.Customers = nilCustomers{CustomerService: d.Customers, err: boom}
		}
	})
	e.wantProblem(e.do("POST", "/v1/auth/logout", "", map[string]any{"refresh_token": "x"}), 500, "internal_error")
	e.wantProblem(e.do("GET", "/v1/auth/me", e.staffTk, nil), 500, "internal_error")
	e.wantProblem(e.do("GET", "/v1/customers", e.staffTk, nil), 500, "internal_error")
}

// authSwitch uses the broken Logout/Me but the real Authenticate/Login (the tokens in
// the test were issued by the real service).
type authSwitch struct {
	httpapi.AuthService
	broken brokenAuth
}

func (a authSwitch) Logout(ctx context.Context, t string) error { return a.broken.Logout(ctx, t) }
func (a authSwitch) Me(ctx context.Context, id string) (auth.User, error) {
	return a.broken.Me(ctx, id)
}

func TestNilListsStillSerializeAsEmptyArrays(t *testing.T) {
	e := newEnv(t, func(o *options) {
		o.mutate = func(d *httpapi.Deps) { d.Customers = nilCustomers{CustomerService: d.Customers} }
	})
	r := e.do("GET", "/v1/customers", e.staffTk, nil)
	if r.status != 200 || r.json(t)["data"] == nil {
		t.Errorf("data must be [] not null: %d %s", r.status, r.body)
	}
}

func TestRateLimitKeyFallsBackWhenThePeerAddressIsNotHostPort(t *testing.T) {
	e := newEnv(t, func(o *options) { o.authLimit = 1 })
	send := func() int {
		req := httptest.NewRequest("POST", "/v1/auth/logout", nil)
		req.RemoteAddr = "pipe"
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec.Code
	}
	if first, second := send(), send(); first == http.StatusTooManyRequests || second != http.StatusTooManyRequests {
		t.Errorf("statuses = %d, %d", first, second)
	}
}
