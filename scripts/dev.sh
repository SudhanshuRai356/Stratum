#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/frontend"

if ! command -v go >/dev/null 2>&1; then
  echo "Error: go is not installed or not in PATH" >&2
  exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "Error: npm is not installed or not in PATH" >&2
  exit 1
fi

if [[ ! -d "$FRONTEND_DIR" ]]; then
  echo "Error: frontend directory not found at $FRONTEND_DIR" >&2
  exit 1
fi

FRONTEND_CMD=(npm run dev)
if [[ ! -f "$FRONTEND_DIR/package.json" ]]; then
  if [[ -x "$FRONTEND_DIR/node_modules/.bin/next" ]]; then
    FRONTEND_CMD=(./node_modules/.bin/next dev)
  else
    FRONTEND_CMD=(npx next dev)
  fi
fi

cleanup() {
  trap - INT TERM EXIT
  if [[ -n "${GO_PID:-}" ]] && kill -0 "$GO_PID" >/dev/null 2>&1; then
    kill "$GO_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "${WEB_PID:-}" ]] && kill -0 "$WEB_PID" >/dev/null 2>&1; then
    kill "$WEB_PID" >/dev/null 2>&1 || true
  fi
  wait >/dev/null 2>&1 || true
}

trap cleanup INT TERM EXIT

echo "Starting Stratum backend (API daemon)..."
(
  cd "$ROOT_DIR"
  go run ./cmd/stratum
) &
GO_PID=$!

echo "Starting frontend dev server..."
(
  cd "$FRONTEND_DIR"
  "${FRONTEND_CMD[@]}"
) &
WEB_PID=$!

echo "Running. Press Ctrl+C to stop both services."
wait "$GO_PID" "$WEB_PID"
