#!/bin/sh
# Runs the Playwright suite in the official image, on the compose network, against the FULL stack
# (nginx + API + PostgreSQL). Needs `make up`. Usage: ./e2e/run.sh [screenshots]
set -eu
cd "$(dirname "$0")/.."
CID=$(docker compose ps -q frontend)
[ -n "$CID" ] || { echo "the stack is not running: make up"; exit 1; }
NET=$(docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}' "$CID")
IMAGE=mcr.microsoft.com/playwright:v1.63.0-noble
VOL=${COMPOSE_PROJECT_NAME:-grbs}_e2e_node_modules
CMD="npm ci --no-fund --no-audit && npx playwright test flow"
[ "${1:-}" = screenshots ] && CMD="npm ci --no-fund --no-audit && SCREENSHOTS=1 npx playwright test screenshots"
docker run --rm --ipc=host --network "$NET" \
  -e ADMIN_EMAIL="${ADMIN_EMAIL:-admin@example.com}" -e ADMIN_PASSWORD="${ADMIN_PASSWORD:-dev-only-admin-password}" \
  -v "$PWD/e2e:/e2e" -v "$VOL:/e2e/node_modules" -v "$PWD/docs/images:/shots" -w /e2e \
  "$IMAGE" sh -c "$CMD"
