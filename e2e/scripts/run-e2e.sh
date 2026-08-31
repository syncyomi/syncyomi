#!/usr/bin/env bash
# Runs the E2E suite. Usage: run-e2e.sh [-run <pattern>] [extra go test args...]
set -euo pipefail

E2E_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_DIR="$(dirname "$E2E_DIR")"

if [ ! -d "$REPO_DIR/web/dist" ]; then
    echo "[run-e2e] building web/dist (server embeds it)"
    (cd "$REPO_DIR/web" && pnpm install --frozen-lockfile && pnpm build)
fi

"$E2E_DIR/scripts/doctor.sh" || { echo "[run-e2e] doctor failed"; exit 1; }

exec go test -C "$REPO_DIR" -tags e2e ./e2e/scenarios/... -v -timeout 60m "$@"
