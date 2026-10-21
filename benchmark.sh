#!/usr/bin/env bash
# Runs the benchmarks in a Linux container (autopprof is Linux-only),
# using the Go version from go.mod. Extra args are passed to `go test`
# and override the defaults, e.g.:
#   ./benchmark.sh -bench 'FlightRecorder|WriteTrace|Event' -count 6 -benchtime 2s
set -euo pipefail

GO_VERSION=$(awk '/^go /{print $2}' go.mod)

docker run --rm -v="$(pwd)":/app -w=/app --cpus=1.5 -m=1000m "golang:${GO_VERSION}" \
	go test -bench . -benchmem -benchtime=10s -run='^$' "$@"
