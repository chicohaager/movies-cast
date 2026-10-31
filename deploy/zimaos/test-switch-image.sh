#!/usr/bin/env bash
# Tests for switch-image.sh, against the repo's ZimaOS compose (the shape the
# installer writes) with one marker line standing in for a real value.
# Run by check.sh; each case prints "ok" or "FAIL" and the script exits 1 on
# the first failure.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
sw="$here/switch-image.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
marker="      - MARKER_VALUE=not-printed-1234"

fixture() { # fixture <file>: repo compose plus the marker line under environment
  awk -v m="$marker" '{ print } /^    environment:/ && !done { print m; done = 1 }' \
    "$here/docker-compose.yml" > "$1"
  [[ "$(grep -c -- "$marker" "$1")" == 1 ]] || { echo "FAIL fixture: marker not placed"; exit 1; }
}
run() { # run <script> <args...>: sets rc and out
  set +e; out="$(sh "$@" 2>&1)"; rc=$?; set -e
}
fail() { echo "FAIL $1"; echo "$out"; exit 1; }

# 1. switch: rc 0, only the image line differs, the marker is not printed
c="$tmp/c1.yml"; fixture "$c"; before="$(sha256sum < "$c")"
run "$sw" "$c" ghcr.io/chicohaager/movies-cast:9.9.9 "$tmp/b1"
[[ $rc == 0 ]] || fail "switch: rc=$rc"
[[ "$(grep -c '^    image: ghcr.io/chicohaager/movies-cast:9.9.9$' "$c")" == 1 ]] || fail "switch: image line not set"
[[ "$out" == *"changed 1, other than the image line 0"* ]] || fail "switch: counts"
[[ "$out" != *MARKER_VALUE* ]] || fail "switch: printed a non-image line"
[[ "$(sha256sum < "$tmp"/b1/*.yml)" == "$before" ]] || fail "switch: backup differs from the original"
[[ "$(stat -c %a "$tmp/b1")" == 700 ]] || fail "switch: backup dir not 0700"
echo "ok 1 switch"

# 2. already on the image: rc 0, nothing changed
run "$sw" "$c" ghcr.io/chicohaager/movies-cast:9.9.9 "$tmp/b2"
[[ $rc == 0 && "$out" == *"changed 0, other than the image line 0"* ]] || fail "same image: rc=$rc"
echo "ok 2 same image"

# 3. two image lines: rc 3, file untouched
c="$tmp/c3.yml"; fixture "$c"; sed -i '0,/^    image: /s//    image: x\n    image: /' "$c"
[[ "$(grep -c '^    image: ' "$c")" == 2 ]] || { echo "FAIL fixture 3: not two image lines"; exit 1; }
before="$(sha256sum < "$c")"
run "$sw" "$c" ghcr.io/chicohaager/movies-cast:9.9.9 "$tmp/b3"
[[ $rc == 3 && "$(sha256sum < "$c")" == "$before" ]] || fail "two image lines: rc=$rc"
echo "ok 3 two image lines"

# 4. no compose: rc 3
run "$sw" "$tmp/none.yml" ghcr.io/chicohaager/movies-cast:9.9.9 "$tmp/b4"
[[ $rc == 3 ]] || fail "no compose: rc=$rc"
echo "ok 4 no compose"

# 5. image name with a sed or shell character: rc 2, file untouched
c="$tmp/c5.yml"; fixture "$c"; before="$(sha256sum < "$c")"
run "$sw" "$c" 'x#y' "$tmp/b5"
[[ $rc == 2 && "$(sha256sum < "$c")" == "$before" ]] || fail "bad image name: rc=$rc"
echo "ok 5 bad image name"

# 6. sabotage: a copy whose edit also changes the marker line must put the
#    backup back (rc 4) and leave the original bytes
sab="$tmp/switch-sabotage.sh"
awk '{ print } /^sed -i "s#\^    image: / { print "sed -i \"s/MARKER_VALUE/CHANGED_VALUE/\" \"$f\"" }' "$sw" > "$sab"
[[ "$(grep -c CHANGED_VALUE "$sab")" == 1 ]] || { echo "FAIL sabotage not placed"; exit 1; }
c="$tmp/c6.yml"; fixture "$c"; before="$(sha256sum < "$c")"
run "$sab" "$c" ghcr.io/chicohaager/movies-cast:9.9.9 "$tmp/b6"
[[ $rc == 4 ]] || fail "sabotage: rc=$rc (want 4)"
[[ "$(sha256sum < "$c")" == "$before" ]] || fail "sabotage: original not put back"
[[ "$out" != *MARKER_VALUE* && "$out" != *CHANGED_VALUE* ]] || fail "sabotage: printed the changed line"
echo "ok 6 another line changed"

echo "switch-image: 6 of 6 cases ok"
