# Everything runs inside Docker (no Go and no Node on the host). `make help` lists the targets.
-include .env
export

COMPOSE := docker compose
GO      := $(COMPOSE) --profile tools run --rm tools
NODE    := $(COMPOSE) --profile tools run --rm --no-deps node
# Every frontend target installs from the lockfile first when node_modules is missing/outdated.
DEPS    := ./scripts/ensure-deps.sh
# Packages measured by the coverage gate (test helpers are excluded, see docs/TESTING.md).
COVERPKG = $$(go list ./internal/... | grep -v -e /testdb -e /repotest | paste -sd, -)

.DEFAULT_GOAL := help
.PHONY: help build test test-unit coverage lint up down clean tidy demo \
	frontend-install frontend-test frontend-coverage frontend-lint frontend-build frontend-types frontend-dev e2e

help: ## Show this help
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-10s %s\n", $$1, $$2}'

build: ## Compile the backend
	$(GO) go build ./...

test-unit: ## Unit tests only, with the race detector (no database needed)
	$(GO) sh -c 'REQUIRE_DB= TEST_DATABASE_URL= go test -race ./...'

test: ## ALL tests: backend (unit + PostgreSQL + concurrency, -race, coverage gate) and frontend (Vitest, coverage gate)
	$(GO) sh -c 'go test -race -count=1 -coverpkg=$(COVERPKG) -coverprofile=coverage.out ./... && sh scripts/coverage-gate.sh coverage.out'
	$(MAKE) frontend-coverage

coverage: test ## Same as test, plus per-function report (backend/coverage.html)
	$(GO) sh -c 'go tool cover -func=coverage.out | tail -n 60 && go tool cover -html=coverage.out -o coverage.html'

lint: ## Backend (gofmt + go vet + staticcheck) and frontend (ESLint + tsc)
	$(GO) sh -c 'test -z "$$(gofmt -l .)" || (gofmt -l . && echo "gofmt: files above need formatting" && exit 1); go vet ./... && staticcheck ./...'
	$(MAKE) frontend-lint

tidy: ## go mod tidy
	$(GO) go mod tidy

up: ## Start everything: PostgreSQL + API + web (app http://localhost:$(or $(WEB_PORT),8096), API docs at /api/docs/)
	$(COMPOSE) up --build -d --wait

down: ## Stop the stack (keeps the database volume)
	$(COMPOSE) --profile tools down

clean: ## Stop the stack, DELETE its volumes/images and the coverage/build files
	$(COMPOSE) --profile tools down -v --rmi local
	rm -rf backend/coverage.out backend/coverage.html frontend/dist frontend/coverage

demo: ## Tour of the running API with curl (needs `make up`, curl and jq)
	API_PORT=$(or $(API_PORT),8095) ./backend/scripts/demo.sh

# ---- frontend (Node 22 in a container; node_modules lives in a named volume) ----
frontend-install: ## npm ci (clean install from the lockfile)
	$(NODE) sh -c 'npm ci --no-fund --no-audit && md5sum package-lock.json | cut -d" " -f1 > node_modules/.lock-hash'

frontend-test: ## Vitest (no coverage)
	$(NODE) sh -c '$(DEPS) && npm test'

frontend-coverage: ## Vitest with coverage; fails below the thresholds in frontend/vite.config.ts
	$(NODE) sh -c '$(DEPS) && npm run coverage'

frontend-lint: ## ESLint + tsc --noEmit + generated API types up to date with openapi.json
	$(NODE) sh -c '$(DEPS) && npm run lint && npm run typecheck && npm run check:api'

frontend-types: ## Regenerate frontend/src/api/schema.d.ts from the backend's openapi.json
	$(NODE) sh -c '$(DEPS) && npm run gen:api'

frontend-build: ## Production bundle (frontend/dist)
	$(NODE) sh -c '$(DEPS) && npm run build'

frontend-dev: ## Vite dev server with HMR on http://localhost:$(or $(DEV_PORT),5180) (needs `make up` for the API)
	$(COMPOSE) up -d --wait api
	$(COMPOSE) --profile tools run --rm --no-deps --service-ports node sh -c '$(DEPS) && npm run dev -- --port 5173'

e2e: ## Playwright end-to-end test against the full stack (needs `make up`; pulls a ~1.5 GB image, see docs/TESTING.md)
	WEB_PORT=$(or $(WEB_PORT),8096) ./e2e/run.sh
