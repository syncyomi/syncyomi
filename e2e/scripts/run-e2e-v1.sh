#!/usr/bin/env bash
# Runs the v1 protocol E2E suite: server-only, no emulators, APKs or Suwayomi.
# Usage: run-e2e-v1.sh [-run <pattern>] [extra go test args...]
set -euo pipefail

E2E_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_DIR="$(dirname "$E2E_DIR")"

if [ ! -d "$REPO_DIR/web/dist" ]; then
    echo "[run-e2e-v1] building web/dist (server embeds it)"
    (cd "$REPO_DIR/web" && pnpm install --frozen-lockfile && pnpm build)
fi

# -count=1 so a green run is always a real run, never Go's cached result
exec go test -C "$REPO_DIR" -tags e2e_v1 -count=1 ./e2e/scenarios/v1/... -v -timeout 10m "$@"
