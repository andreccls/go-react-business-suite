// Package httpapi is the HTTP adapter: routing, middleware, JSON and problem
// details, and the embedded OpenAPI document + Swagger UI. Handlers are thin: they
// decode, call a service, and encode.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/andreccls/go-react-business-suite/backend/internal/auth"
	"github.com/andreccls/go-react-business-suite/backend/internal/booking"
	"github.com/andreccls/go-react-business-suite/backend/internal/catalog"
	"github.com/andreccls/go-react-business-suite/backend/internal/customer"
	"github.com/andreccls/go-react-business-suite/backend/internal/dashboard"
	"github.com/andreccls/go-react-business-suite/backend/internal/ratelimit"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// The services the handlers need, declared here, at the consumer.
type (
	CatalogService interface {
		Create(ctx context.Context, in catalog.Input) (catalog.Item, error)
		Get(ctx context.Context, id string) (catalog.Item, error)
		List(ctx context.Context, f catalog.Filter) ([]catalog.Item, int, error)
		Update(ctx context.Context, id string, p catalog.Patch) (catalog.Item, error)
		Delete(ctx context.Context, id string) error
	}
	CustomerService interface {
		Create(ctx context.Context, in customer.Input) (customer.Customer, error)
		Get(ctx context.Context, id string) (customer.Customer, error)
		List(ctx context.Context, f customer.Filter) ([]customer.Customer, int, error)
		Update(ctx context.Context, id string, p customer.Patch) (customer.Customer, error)
		Delete(ctx context.Context, id string) error
	}
	BookingService interface {
		Create(ctx context.Context, in booking.Input) (booking.Appointment, error)
		Get(ctx context.Context, id string) (booking.Appointment, error)
		List(ctx context.Context, q booking.Query) ([]booking.Appointment, int, error)
		SetStatus(ctx context.Context, id string, to booking.Status) (booking.Appointment, error)
	}
	DashboardService interface {
		Summary(ctx context.Context, from, to string) (dashboard.Summary, error)
		Daily(ctx context.Context, from, to string) (dashboard.DailySeries, error)
		TopServices(ctx context.Context, from, to string, limit int) (dashboard.Ranking, error)
		UpcomingAppointments(ctx context.Context, limit int) ([]booking.Appointment, error)
	}
	AuthService interface {
		CreateUser(ctx context.Context, email, password string, role auth.Role) (auth.User, error)
		Login(ctx context.Context, email, password string) (auth.Tokens, error)
		Refresh(ctx context.Context, refreshToken string) (auth.Tokens, error)
		Logout(ctx context.Context, refreshToken string) error
		Me(ctx context.Context, userID string) (auth.User, error)
		Authenticate(token string) (auth.Principal, error)
	}
)

// Deps are the collaborators of the API.
type Deps struct {
	Catalog        CatalogService
	Customers      CustomerService
	Bookings       BookingService
	Dashboard      DashboardService
	Auth           AuthService
	Ready          func(context.Context) error // readiness probe (database ping)
	Logger         *slog.Logger
	AuthLimiter    *ratelimit.Limiter // per client IP, /v1/auth/*
	APILimiter     *ratelimit.Limiter // per user, everything else under /v1
	RequestTimeout time.Duration
	CORSOrigins    []string // allowed browser origins; "*" = any; empty = CORS off
}

type server struct {
	Deps
	log *slog.Logger
}

// Access says who may call a route.
type Access int

const (
	Public  Access = iota // no token
	AnyUser               // any valid token (admin or staff)
	AdminOnly
)

// route is one API operation. This table is the single source of truth for the
// router; the OpenAPI test compares it with openapi.json, so the two cannot drift.
type route struct {
	method, path string
	access       Access
	limited      bool // /v1/auth/* : rate-limited per IP
	handler      http.HandlerFunc
}

func (s *server) routes() []route {
	return []route{
		{"GET", "/healthz", Public, false, s.healthz},
		{"GET", "/readyz", Public, false, s.readyz},

		{"POST", "/v1/auth/login", Public, true, s.login},
		{"POST", "/v1/auth/refresh", Public, true, s.refresh},
		{"POST", "/v1/auth/logout", Public, true, s.logout},
		{"GET", "/v1/auth/me", AnyUser, false, s.me},
		{"POST", "/v1/users", AdminOnly, false, s.createUser},

		{"POST", "/v1/services", AnyUser, false, s.createService},
		{"GET", "/v1/services", AnyUser, false, s.listServices},
		{"GET", "/v1/services/{id}", AnyUser, false, s.getService},
		{"PATCH", "/v1/services/{id}", AnyUser, false, s.patchService},
		{"DELETE", "/v1/services/{id}", AdminOnly, false, s.deleteService},

		{"POST", "/v1/customers", AnyUser, false, s.createCustomer},
		{"GET", "/v1/customers", AnyUser, false, s.listCustomers},
		{"GET", "/v1/customers/{id}", AnyUser, false, s.getCustomer},
		{"PATCH", "/v1/customers/{id}", AnyUser, false, s.patchCustomer},
		{"DELETE", "/v1/customers/{id}", AdminOnly, false, s.deleteCustomer},

		{"POST", "/v1/appointments", AnyUser, false, s.createAppointment},
		{"GET", "/v1/appointments", AnyUser, false, s.listAppointments},
		{"GET", "/v1/appointments/{id}", AnyUser, false, s.getAppointment},
		{"PATCH", "/v1/appointments/{id}/status", AnyUser, false, s.setAppointmentStatus},

		{"GET", "/v1/dashboard/summary", AnyUser, false, s.dashboardSummary},
		{"GET", "/v1/dashboard/daily", AnyUser, false, s.dashboardDaily},
		{"GET", "/v1/dashboard/top-services", AnyUser, false, s.dashboardTopServices},
		{"GET", "/v1/dashboard/upcoming", AnyUser, false, s.dashboardUpcoming},
	}
}

// Operation describes one route for documentation tests.
type Operation struct {
	Method, Path string
	Access       Access
	RateLimited  bool
}

// Operations lists the routes the API serves (excluding /docs and /openapi.json).
func Operations() []Operation {
	var ops []Operation
	for _, rt := range (&server{}).routes() {
		ops = append(ops, Operation{rt.method, rt.path, rt.access, rt.limited || rt.access != Public})
	}
	return ops
}

// New builds the full handler: routes, docs and the middleware chain.
func New(d Deps) http.Handler {
	s := &server{Deps: d, log: d.Logger}
	mux := http.NewServeMux()
	for _, rt := range s.routes() {
		pattern := rt.method + " " + rt.path
		mux.Handle(pattern, named(pattern, s.protect(rt)))
	}
	mountDocs(mux)
	mux.Handle("/", named("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, http.StatusNotFound, "route_not_found", "No such route.", nil)
	})))

	// Outermost first: observe (ID, log) -> CORS (preflight answers here, before auth
	// and rate limits) -> recover -> limits (body, timeout) -> mux.
	return s.observe(s.cors(s.recoverer(s.limits(mux))))
}

// protect wraps a handler with the rate limit and authentication its route needs.
func (s *server) protect(rt route) http.Handler {
	var h http.Handler = rt.handler
	switch rt.access {
	case AdminOnly:
		h = requireRole(auth.RoleAdmin, h)
		fallthrough
	case AnyUser:
		h = s.authenticate(rateLimit(s.APILimiter, byUser, h))
	default:
		if rt.limited {
			h = rateLimit(s.AuthLimiter, byIP, h)
		}
	}
	return h
}
