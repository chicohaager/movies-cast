#!/usr/bin/env bash
# Everything that must pass before a build leaves this machine:
#
#   ./check.sh            go vet, go tests, browser tests (e2e/), public-data scan
#   ./check.sh --no-e2e   without the browser tests (no Node/Chromium at hand)
#
# deploy/zimaos/build-on-host.sh and deploy/testers/make-package.sh run it
# first. Each step prints its name; the first failure stops the script.
set -euo pipefail

repo="$(cd "$(dirname "$0")" && pwd)"
cd "$repo"
e2e=1
[[ "${1:-}" == "--no-e2e" ]] && e2e=0

echo "== check 1/4: gofmt + go vet"
unformatted="$(gofmt -l cmd internal)"
if [[ -n "$unformatted" ]]; then
  echo "not gofmt-formatted:" >&2
  echo "$unformatted" >&2
  exit 1
fi
go vet ./...

echo "== check 2/4: go test"
go test -count=1 ./...

echo "== check 3/4: browser tests"
if [[ "$e2e" == 1 ]]; then
  if [[ ! -d e2e/node_modules ]]; then
    (cd e2e && npm ci --no-audit --no-fund)
  fi
  (cd e2e && npx playwright test)
else
  echo "skipped (--no-e2e)"
fi

echo "== check 4/4: no private data in the tree"
scanner="$HOME/.claude/bin/check-public-data.py"
if [[ -f "$scanner" ]]; then
  python3 "$scanner" "$repo"
else
  echo "skipped ($scanner not on this machine)"
fi

echo "== check: all passed"
