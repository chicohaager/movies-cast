#!/bin/sh
# Point the one "    image:" line of an installed ZimaOS compose at another image
# and prove that nothing else changed. Runs on the host as root
# (build-on-host.sh --switch); tested by test-switch-image.sh.
#
#   switch-image.sh <compose> <image> <backup-dir>
#
# The installed compose holds the real values (passwords, tokens), so only
# counts and the image lines are printed, never other lines. Lines are compared
# with awk, not diff: ZimaOS had no diff until the admin tools brought one
# (the test NAS, 2026-10-31).
#
# Exit: 0 switched (or already on <image>); 2 bad arguments; 3 no compose or
# not exactly one image line, nothing changed; 4 another line changed, the
# backup was put back.
set -eu

[ $# -eq 3 ] || { echo "usage: switch-image.sh <compose> <image> <backup-dir>"; exit 2; }
f="$1"; image="$2"; bdir="$3"
case "$image" in
  "" | *[!A-Za-z0-9./:_@-]*) echo "switch: image name not allowed: $image"; exit 2 ;;
esac
[ -f "$f" ] || { echo "switch: no compose at $f"; exit 3; }
n=$(grep -c '^    image: ' "$f" || true)
[ "$n" = 1 ] || { echo "switch: $n image lines in $f, nothing changed"; exit 3; }

umask 077
mkdir -p "$bdir"
b="$bdir/docker-compose-$(date +%Y%m%d-%H%M%S).yml"
cp -p "$f" "$b"
echo "switch: backup $b"

sed -i "s#^    image: .*#    image: $image#" "$f"

lines_b=$(wc -l < "$b")
lines_f=$(wc -l < "$f")
changed=$(awk 'NR==FNR { a[FNR] = $0; next } a[FNR] != $0 { c++ } END { print c + 0 }' "$b" "$f")
other=$(awk 'NR==FNR { a[FNR] = $0; next }
  a[FNR] != $0 && (a[FNR] !~ /^    image: / || $0 !~ /^    image: /) { c++ }
  END { print c + 0 }' "$b" "$f")
echo "switch: lines $lines_b -> $lines_f, changed $changed, other than the image line $other"
grep '^    image: ' "$b" | sed 's/^ */switch: before /'
grep '^    image: ' "$f" | sed 's/^ */switch: after  /'

if [ "$lines_b" != "$lines_f" ] || [ "$other" != 0 ]; then
  cp -p "$b" "$f"
  echo "switch: another line changed, backup put back"
  exit 4
fi
