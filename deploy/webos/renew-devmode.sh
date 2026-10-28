#!/bin/sh
# Keeps the LG TV's Developer Mode session alive, so the movies-cast TV app is
# not uninstalled. Per LG's docs the session is time-limited, an expired
# session cannot be extended, and apps are removed after a reboot past expiry.
#
# Mechanism (taken from webosbrew/dev-manager-desktop renew-script, checked
# 2026-10-28; first manual run answered {"result":"success"}):
#   1. read the session token from /var/luna/preferences/devmode_enabled over SSH
#   2. GET https://developer.lge.com/secure/ResetDevModeSession.dev?sessionToken=<token>
# The token is cached, so renewal also works while the TV is off.
#
# Config: DEVMODE_DIR (default /DATA/AppData/movies-cast-devmode) holding
#   tv.env      TV_HOST=…, TV_PORT=9922
#   tv_key      the TV's SSH key (encrypted, from ares-novacom --getkey)
#   passphrase  its passphrase (the one shown by the Developer Mode app)
#   token       cache, written by this script
# Never prints the token. Exit 0 only on {"result":"success"}.
set -u
umask 077
dir="${DEVMODE_DIR:-/DATA/AppData/movies-cast-devmode}"
# shellcheck disable=SC1091
. "$dir/tv.env"
work="$(mktemp -d)" || exit 1
trap 'rm -rf "$work"' EXIT

valid() { case "$1" in '' | *[!0-9a-zA-Z]*) return 1 ;; *) return 0 ;; esac; }

token=""
cp "$dir/tv_key" "$work/key"
if ssh-keygen -p -P "$(cat "$dir/passphrase")" -N '' -f "$work/key" >/dev/null 2>&1; then
	t="$(ssh -n -i "$work/key" -p "${TV_PORT:-9922}" -o BatchMode=yes -o ConnectTimeout=5 \
		-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
		-o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa \
		"prisoner@$TV_HOST" cat /var/luna/preferences/devmode_enabled 2>/dev/null)"
	if valid "$t"; then
		token="$t"
		printf '%s\n' "$token" >"$dir/token"
		echo "token read from TV $TV_HOST"
	else
		echo "TV $TV_HOST not reachable or no token; using cache"
	fi
else
	echo "cannot decrypt TV key (passphrase wrong?); using cache" >&2
fi
if [ -z "$token" ] && [ -r "$dir/token" ]; then
	token="$(cat "$dir/token")"
fi
if ! valid "$token"; then
	echo "FEHLER: no valid token (TV off and no cache)" >&2
	exit 1
fi

resp="$(curl --max-time 15 -sS "https://developer.lge.com/secure/ResetDevModeSession.dev?sessionToken=$token")"
rc=$?
if [ $rc -ne 0 ]; then
	echo "FEHLER: renewal request failed (curl rc=$rc)" >&2
	exit 1
fi
if printf '%s' "$resp" | python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(0 if isinstance(d,dict) and d.get("result")=="success" else 1)' 2>/dev/null; then
	echo "renewed: result=success"
	exit 0
fi
# Only named fields; the answer is not printed whole.
printf '%s' "$resp" | python3 -c 'import json,sys
try:
    d=json.load(sys.stdin); print("FEHLER: renewal refused:", {k:d.get(k) for k in ("result","errorCode","errorMsg")})
except Exception:
    print("FEHLER: renewal answer is not JSON")' >&2
exit 1
