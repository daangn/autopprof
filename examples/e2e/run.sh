#!/usr/bin/env bash
# Run the autopprof e2e test inside a cgroup-constrained Linux container.
#
# Required env:
#   SLACK_TOKEN       — Slack bot token with files:write.
#   SLACK_CHANNEL_ID  — destination channel ID (not name).
#
# Optional env:
#   SLACK_APP         — "<app>" segment in built-in filenames (default "autopprof-e2e").
#   E2E_DURATION      — e.g. "180s" (default). Needs ~2 min for CPU snapshot warmup.
#
# Runs from the repo root.

set -euo pipefail

if [[ -z "${SLACK_TOKEN:-}" || -z "${SLACK_CHANNEL_ID:-}" ]]; then
  echo "SLACK_TOKEN and SLACK_CHANNEL_ID env vars are required" >&2
  exit 1
fi

cd "$(git rev-parse --show-toplevel)"

exec docker run --rm \
  --cpus=1 -m=512m \
  -e SLACK_TOKEN \
  -e SLACK_CHANNEL_ID \
  -e SLACK_APP="${SLACK_APP:-}" \
  -e E2E_DURATION="${E2E_DURATION:-}" \
  -v "$(pwd):/app" \
  -w /app/examples \
  golang:1.22 \
  go run ./e2e
