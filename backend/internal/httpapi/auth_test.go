package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestLoginMeAndRefreshFlow(t *testing.T) {
	e := newEnv(t)

	e.wantProblem(e.do("POST", "/v1/auth/login", "", map[string]any{"email": adminEmail, "password": "wrong-password"}), 401, "invalid_credentials")
	e.wantProblem(e.do("POST", "/v1/auth/login", "", map[string]any{"email": "ghost@example.com", "password": "whatever-pass"}), 401, "invalid_credentials")

	login := e.do("POST", "/v1/auth/login", "", map[string]any{"email": " Admin@Example.com ", "password": adminPass})
	if login.status != 200 {
		t.Fatalf("login: %d %s", login.status, login.body)
	}
	tok := login.json(t)
	if tok["token_type"] != "Bearer" || tok["expires_in"] != 900.0 {
		t.Errorf("token response: %v", tok)
	}
	access, refresh := tok["access_token"].(string), tok["refresh_token"].(string)

	me := e.do("GET", "/v1/auth/me", access, nil)
	if me.status != 200 || me.json(t)["email"] != adminEmail || me.json(t)["role"] != "admin" {
		t.Errorf("me: %d %s", me.status, me.body)
	}
	if strings.Contains(string(me.body), "password") {
		t.Error("/me must not expose password material")
	}

	rotated := e.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": refresh})
	if rotated.status != 200 || rotated.json(t)["refresh_token"] == refresh {
		t.Fatalf("refresh: %d %s", rotated.status, rotated.body)
	}
	e.wantProblem(e.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": refresh}), 401, "invalid_token")
	// The replay above revoked the whole family: the rotated token is dead too.
	e.wantProblem(e.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": rotated.json(t)["refresh_token"]}), 401, "invalid_token")

	fresh := e.do("POST", "/v1/auth/login", "", map[string]any{"email": adminEmail, "password": adminPass}).json(t)
	if r := e.do("POST", "/v1/auth/logout", "", map[string]any{"refresh_token": fresh["refresh_token"]}); r.status != 204 || len(r.body) != 0 {
		t.Errorf("logout: %d %q", r.status, r.body)
	}
	e.wantProblem(e.do("POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": fresh["refresh_token"]}), 401, "invalid_token")
}

func TestAuthenticationAndRoles(t *testing.T) {
	e := newEnv(t)
	r := e.do("GET", "/v1/services", "", nil)
	e.wantProblem(r, 401, "missing_token")
	if r.header.Get("WWW-Authenticate") == "" {
		t.Error("401 must carry WWW-Authenticate")
	}
	e.wantProblem(e.do("GET", "/v1/services", "garbage.token.value", nil), 401, "invalid_token")
	e.wantProblem(e.do("GET", "/v1/services", "", nil, "Authorization", "Basic abc"), 401, "missing_token")
	e.wantProblem(e.do("GET", "/v1/auth/me", "", nil), 401, "missing_token")
	e.wantProblem(e.do("POST", "/v1/users", e.staffTk, map[string]any{"email": "x@example.com", "password": "long-enough-pass", "role": "staff"}), 403, "forbidden")
}

func TestAdminCreatesUsers(t *testing.T) {
	e := newEnv(t)
	body := map[string]any{"email": "New@Example.com", "password": "long-enough-pass", "role": "staff"}
	r := e.do("POST", "/v1/users", e.adminTk, body)
	if r.status != 201 || r.json(t)["role"] != "staff" || r.json(t)["email"] != "new@example.com" {
		t.Fatalf("create user: %d %s", r.status, r.body)
	}
	if strings.Contains(string(r.body), "password") {
		t.Error("the response must not echo password material")
	}
	e.wantProblem(e.do("POST", "/v1/users", e.adminTk, body), 409, "email_taken")
	e.wantProblem(e.do("POST", "/v1/users", e.adminTk, map[string]any{"email": "bad", "password": "short", "role": "staff"}), 422, "validation_failed")
	p := e.wantProblem(e.do("POST", "/v1/users", e.adminTk, map[string]any{"email": "r@example.com", "password": "long-enough-pass", "role": "root"}), 422, "validation_failed")
	if f := p["errors"].([]any)[0].(map[string]any)["field"]; f != "role" {
		t.Errorf("field = %v", f)
	}
	// The new user can log in and is a staff member (cannot delete).
	login := e.do("POST", "/v1/auth/login", "", map[string]any{"email": "new@example.com", "password": "long-enough-pass"})
	svc := e.newService("Cut", 30, 5000)
	e.wantProblem(e.do("DELETE", "/v1/services/"+svc["id"].(string), login.json(t)["access_token"].(string), nil), 403, "forbidden")
}

func TestRateLimiting(t *testing.T) {
	t.Run("auth endpoints per IP", func(t *testing.T) {
		e := newEnv(t, func(o *options) { o.authLimit = 3 })
		for i := 0; i < 3; i++ {
			e.do("POST", "/v1/auth/login", "", map[string]any{"email": "x@example.com", "password": "whatever-pass"})
		}
		r := e.do("POST", "/v1/auth/login", "", map[string]any{"email": "x@example.com", "password": "whatever-pass"})
		e.wantProblem(r, 429, "rate_limited")
		if r.header.Get("Retry-After") == "" {
			t.Error("429 must carry Retry-After")
		}
	})
	t.Run("API per user, independent between users", func(t *testing.T) {
		e := newEnv(t, func(o *options) { o.apiLimit = 2 })
		for i := 0; i < 2; i++ {
			if r := e.do("GET", "/v1/services", e.staffTk, nil); r.status != http.StatusOK {
				t.Fatalf("request %d: %d", i, r.status)
			}
		}
		e.wantProblem(e.do("GET", "/v1/services", e.staffTk, nil), 429, "rate_limited")
		if r := e.do("GET", "/v1/services", e.adminTk, nil); r.status != http.StatusOK {
			t.Errorf("another user must have its own budget: %d", r.status)
		}
	})
}
