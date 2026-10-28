#!/bin/sh
# Push gate: the public-data scan must say "clean" — BLOCK *and* REVIEW stop
# the push. 2026-10-28: a REVIEW finding (an example address in the README)
# went out with 0d4f9c1 because the scan ran in the same command as the push
# without gating it.
#
# Install once per clone:  ln -s ../../deploy/tools/pre-push.sh .git/hooks/pre-push
out=$(python3 "$HOME/.claude/bin/check-public-data.py" "$(git rev-parse --show-toplevel)" 2>&1)
if printf '%s\n' "$out" | grep -q '^check-public-data: clean'; then
	exit 0
fi
printf '%s\n' "$out" | tail -n 12 >&2
echo "pre-push: public-data scan not clean — decide each finding (.public-data-allow), then push again" >&2
exit 1
