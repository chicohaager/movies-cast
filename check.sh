#!/usr/bin/env bash
# Everything that must pass before a build leaves this machine:
#
#   ./check.sh               go vet, go tests (+ switch-image.sh, the device and Movies tools), browser tests (e2e/), Android app, public-data scan
#   ./check.sh --no-e2e      without the browser tests (no Node/Chromium at hand)
#   ./check.sh --no-android  without the Android app (no Android SDK at hand)
#
# deploy/zimaos/build-on-host.sh and deploy/testers/make-package.sh run it
# first. Each step prints its name; the first failure stops the script.
set -euo pipefail

repo="$(cd "$(dirname "$0")" && pwd)"
cd "$repo"
e2e=1
android=1
for arg in "$@"; do
  case "$arg" in
    --no-e2e) e2e=0 ;;
    --no-android) android=0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

echo "== check 1/5: gofmt + go vet"
unformatted="$(gofmt -l cmd internal)"
if [[ -n "$unformatted" ]]; then
  echo "not gofmt-formatted:" >&2
  echo "$unformatted" >&2
  exit 1
fi
go vet ./...

echo "== check 2/5: go test"
go test -count=1 ./...
deploy/zimaos/test-switch-image.sh
deploy/tools/test-pick-device.sh
# the tools for devices and for Movies, each against a fake (adb, server) with made-up values
python3 deploy/tools/test_android_screen.py >/dev/null || { python3 deploy/tools/test_android_screen.py | grep -v "^ok"; exit 1; }
python3 deploy/tools/test_movies_probe.py >/dev/null || { python3 deploy/tools/test_movies_probe.py | grep -v "^ok"; exit 1; }

echo "== check 3/5: browser tests"
if [[ "$e2e" == 1 ]]; then
  if [[ ! -d e2e/node_modules ]]; then
    (cd e2e && npm ci --no-audit --no-fund)
  fi
  (cd e2e && npx playwright test)
else
  echo "skipped (--no-e2e)"
fi

echo "== check 4/5: Android app (tests, build, FFmpeg decoder inside, 16 KB pages)"
if [[ "$android" == 1 ]]; then
  deploy/tools/check-android.sh
else
  echo "skipped (--no-android)"
fi

echo "== check 5/5: no private data in the tree"
scanner="$HOME/.claude/bin/check-public-data.py"
if [[ -f "$scanner" ]]; then
  python3 "$scanner" "$repo"
else
  echo "skipped ($scanner not on this machine)"
fi

echo "== check: all passed"
