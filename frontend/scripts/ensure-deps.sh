#!/bin/sh
# Installs from the lockfile only when node_modules is missing or older than package-lock.json
# (a clean clone, or after a dependency change). Used by the Makefile; CI runs `npm ci` itself.
set -eu
want=$(md5sum package-lock.json | cut -d' ' -f1)
if [ "$(cat node_modules/.lock-hash 2>/dev/null || true)" != "$want" ]; then
  npm ci --no-fund --no-audit
  echo "$want" > node_modules/.lock-hash
fi
