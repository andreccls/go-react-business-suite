# go-react-business-suite

> ⚠️ **SAMPLE / REFERENCE CODE — NOT A PRODUCTION-READY PRODUCT.** The studio and the data are fictional; every secret in
> `docker-compose.yml`/`.env.example` is development-only. Known limitations: [README.md](README.md#limitações-conhecidas-e-próximos-passos) (in Portuguese).

A small-business suite for a fictional single-provider studio (Go API + React SPA): **service catalog** (price in cents, duration, active flag), **customers**,
**appointments** (end time derived from the service duration; price/duration/name frozen as a snapshot; no overlaps, no past or inactive-service bookings;
`scheduled → completed | cancelled | no_show`) and a **results dashboard** (KPIs, daily series, top services, upcoming appointments).

- **Stack:** Go 1.24 (stdlib `net/http`, `log/slog`), PostgreSQL 16 via `pgx`, 4 direct dependencies, distroless non-root image.
- **No double booking, ever:** a PostgreSQL `EXCLUDE USING gist (tstzrange(starts_at, ends_at, '[)') WITH &&)` constraint, proven by concurrency tests
  on a real database (30 simultaneous requests for one slot → exactly one `201`, 29 × `409`). [ADR 0002](docs/adr/0002-non-overlapping-agenda.md).
- **Auth:** short-lived JWT + rotating single-use refresh tokens, `admin`/`staff` roles, per-IP/per-user rate limiting, release mode refuses weak secrets.
- **API contract for the frontend:** an embedded OpenAPI 3.0 document + Swagger UI at `/docs/`; a test fails if it ever diverges from the router, and every
  response in every API test is validated against its schema. CORS is configurable; in production nginx proxies `/api` (no CORS needed).
- **Tests:** 227 test cases, `-race`, 97.7% total coverage (domain and services 100%, gate ≥ 90%); the same contract suite runs on the in-memory fake and on real
  PostgreSQL. [docs/TESTING.md](docs/TESTING.md).
- **Frontend:** React 19 + TypeScript (strict) + Vite; TanStack Query for server state; a thin fetch client with **single-flight token refresh** (access token in memory, refresh token in
  `sessionStorage`, [ADR 0008](docs/adr/0008-token-storage.md)); types **generated from `openapi.json`** and verified in CI; pt-BR formatting in the business time zone; light/dark by
  `prefers-color-scheme`; responsive tables that become cards on phones; own SVG chart with a data table; accessible dialogs, labels, skip link. Served by **non-root nginx** with a strict CSP
  and a same-origin `/api` proxy ([ADR 0009](docs/adr/0009-nginx-proxy-csp.md)). Bundle: 345 kB JS (**106 kB gzip**).
- **Frontend tests:** 188 Vitest + Testing Library + MSW cases (fake API typed by the contract), **99.9% lines / 98.1% branches** (gate ≥ 90%, `lib/` and the HTTP client ≥ 95%), ESLint (jsx-a11y) and
  `tsc` clean; **4 Playwright E2E tests** against the full stack (booking with a `409` conflict, staff permissions, keyboard-only, no console errors).
- **Docs:** [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and 10 ADRs in [docs/adr](docs/adr) (stdlib vs framework, non-overlap, snapshot, dashboard aggregations with
  `EXPLAIN`, auth, server state, generated API types, token storage, nginx/CSP, lightweight dependencies).

```bash
make up      # PostgreSQL + API + frontend -> app http://localhost:8096 (API :8095, Swagger UI at /docs/ or /api/docs/)
make demo    # curl tour (empty agenda); checks the dashboard KPIs against a manual computation
make test    # backend (unit + PostgreSQL + concurrency, -race) and frontend (Vitest), both with coverage gates
make lint    # gofmt + go vet + staticcheck, ESLint + tsc + generated-types check
make e2e     # Playwright against the full stack (pulls the official ~1.5 GB image)
```

Docker is the only requirement (no Go and no Node on the host); a clean clone works without a `.env`. Sign in at <http://localhost:8096> with the dev admin from `.env.example`
(`admin@example.com` / `dev-only-admin-password`). Honest limitations (XSS can read the refresh token, shared login rate limit behind nginx, build-time time zone…): see
[README.md](README.md#limitações-conhecidas-e-próximos-passos) (Portuguese). MIT licensed.
