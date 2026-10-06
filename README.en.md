# go-react-business-suite

> ⚠️ **SAMPLE / REFERENCE CODE — NOT A PRODUCTION-READY PRODUCT.** The studio and the data are fictional; every secret in
> `docker-compose.yml`/`.env.example` is development-only. Known limitations: [README.md](README.md#limitações-conhecidas-e-próximos-passos) (in Portuguese).

> 🚧 **Stage 1 of 2:** the Go backend is complete and verified; the React frontend is next (`frontend/` is reserved, the compose service is commented out).

A small-business suite for a fictional single-provider studio: **service catalog** (price in cents, duration, active flag), **customers**,
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
- **Docs:** [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and 5 ADRs in [docs/adr](docs/adr) (stdlib vs framework, non-overlap, snapshot, dashboard aggregations with
  `EXPLAIN`, auth).

```bash
make up      # PostgreSQL + API -> http://localhost:8095 (Swagger UI at /docs/)
make demo    # curl tour; ends by checking the dashboard KPIs against a manual computation
make test    # unit + PostgreSQL + concurrency, -race, coverage gate
make lint    # gofmt + go vet + staticcheck
```

Docker is the only requirement (no Go on the host); a clean clone works without a `.env`. MIT licensed.
