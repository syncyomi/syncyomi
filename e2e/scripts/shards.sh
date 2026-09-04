#!/usr/bin/env bash
# Prints the CI shard matrix for the Android e2e suite as JSON and fails if any
# scenario in e2e/scenarios is not in exactly one shard.
set -euo pipefail

E2E_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

shards='{"include":[
  {"name":"devices",    "run":"TestS13_|TestS3_|TestS4_|TestS6_|TestS8_"},
  {"name":"categories", "run":"TestS10_|TestS12_|TestS11_|TestS2_"},
  {"name":"suwayomi",   "run":"TestS15_|TestS7_|TestS14_|TestS1_|TestS5_"}
]}'

status=0
for test in $(grep -hoE '^func Test[A-Za-z0-9_]+' "$E2E_DIR"/scenarios/*_test.go | cut -d' ' -f2 | grep -vx TestMain); do
    hits=0
    while read -r pattern; do
        grep -qE "$pattern" <<<"$test" && hits=$((hits + 1))
    done < <(jq -r '.include[].run' <<<"$shards")
    if [ "$hits" != 1 ]; then
        echo "$test is in $hits shards, want exactly 1" >&2
        status=1
    fi
done
[ "$status" = 0 ] || exit "$status"

jq -c . <<<"$shards"
