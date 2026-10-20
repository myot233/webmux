#!/usr/bin/env bash
set -uo pipefail

# Both suites must finish and both exit codes participate in the job result.
# Playwright global setup builds the embedded app before starting any workers.
# The workflow builds dist first so Go's embed has the current assets too.
go test -race -parallel=4 ./... &
race_pid=$!
pnpm --dir web test:e2e &
e2e_pid=$!
trap 'kill "$race_pid" "$e2e_pid" 2>/dev/null || true; wait; exit 130' INT TERM
race_status=0
wait "$race_pid" || race_status=$?
e2e_status=0
wait "$e2e_pid" || e2e_status=$?
printf 'Go race exit: %s; Playwright exit: %s\n' "$race_status" "$e2e_status"
(( race_status == 0 && e2e_status == 0 ))
