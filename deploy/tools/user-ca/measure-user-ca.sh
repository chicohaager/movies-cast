#!/usr/bin/env bash
# Does a certificate the person installed in Android's settings count for their server, and only the
# one they logged in with?
#
#   deploy/tools/user-ca/measure-user-ca.sh [serial]
#
# Needs an emulator with root (an image "Google APIs", not "Google Play"; android/README.md) whose
# system language is English, and the debug build (cd android && ./gradlew :app:assembleDebug).
# Makes two self-signed certificates for 127.0.0.1 ("own" and "other"), serves fake-https.py with one
# of them on this machine and lets the device reach it as 127.0.0.1 (adb reverse). Then, from the
# app's login page and screens:
#   1 no certificate installed, log in          -> "does not trust", no login at the server
#   2 "own" installed, log in                   -> the library, one login at the server
#   3 "other" installed too, the server shows it; the app started again with its login
#                                               -> "is not the one it had", no request reaches the server
#   4 both removed, log in afresh               -> "does not trust", no login at the server
# The app's data on the device is cleared before each login and at the end.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
ANDROID_SERIAL="$("$repo/deploy/tools/pick-device.sh" ${1:+"$1"})"
export ANDROID_SERIAL
app=io.github.chicohaager.moviescast.debug
apk="$repo/android/app/build/outputs/apk/debug/app-debug.apk"
port=18443
[[ -f "$apk" ]] || { echo "no debug build: cd android && ./gradlew :app:assembleDebug" >&2; exit 1; }
if [[ "$(adb shell getprop ro.build.type | tr -d '\r')" == user ]]; then
  echo "this device has no root (a 'user' build); use an emulator image 'Google APIs'" >&2
  exit 1
fi
adb root >/dev/null
adb wait-for-device
[[ "$(adb shell id -u | tr -d '\r')" == 0 ]] || { echo "adb root did not give root" >&2; exit 1; }
if ss -ltn | grep -q ":$port "; then echo "port $port is in use on this machine" >&2; exit 1; fi

tmp="$(mktemp -d)"
server=""
stores=()
cleanup() {
  if [[ -n "$server" ]]; then kill "$server" 2>/dev/null || true; fi
  for s in "${stores[@]}"; do adb shell rm -f "$s" || true; done
  adb reverse --remove "tcp:$port" 2>/dev/null || true
  adb shell pm clear "$app" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

make_cert() { # make_cert <name>: $tmp/<name>.pem and its key, and the place it takes in the user store
  openssl req -x509 -newkey rsa:2048 -nodes -keyout "$tmp/$1.key" -out "$tmp/$1.pem" -days 2 \
    -subj "/CN=invented test server $1" -addext "subjectAltName=IP:127.0.0.1" \
    -addext "basicConstraints=critical,CA:TRUE" 2>"$tmp/openssl.log"
  # Android names a CA file in its user store by the old subject hash and ".0" (what the settings write).
  stores+=("/data/misc/user/0/cacerts-added/$(openssl x509 -in "$tmp/$1.pem" -noout -subject_hash_old).0")
}
store_of() { openssl x509 -in "$tmp/$1.pem" -noout -subject_hash_old | sed 's|^|/data/misc/user/0/cacerts-added/|; s|$|.0|'; }
install_cert() { # install_cert <name>: as the settings put it
  local s
  s="$(store_of "$1")"
  adb push "$tmp/$1.pem" /data/local/tmp/mc-ca.pem >/dev/null
  adb shell "mkdir -p /data/misc/user/0/cacerts-added && cp /data/local/tmp/mc-ca.pem $s && rm /data/local/tmp/mc-ca.pem && chown system:system $s && chmod 644 $s && restorecon $s" 2>>"$tmp/restorecon.log"
}
serve() { # serve <name>: the fake server with that certificate (a running one is stopped first)
  if [[ -n "$server" ]]; then kill "$server"; wait "$server" 2>/dev/null || true; server=""; fi
  for _ in $(seq 1 20); do
    if ! ss -ltn | grep -q ":$port "; then break; fi
    sleep 0.2
  done
  /usr/bin/python3 "$here/fake-https.py" "$tmp/$1.pem" "$tmp/$1.key" "$port" 2>"$tmp/server.log" &
  server=$!
  for _ in $(seq 1 20); do
    if ss -ltn | grep -q ":$port "; then return 0; fi
    sleep 0.2
  done
  echo "the fake server did not start:" >&2
  cat "$tmp/server.log" >&2
  exit 1
}

screen() { python3 "$repo/deploy/tools/android-screen.py" "$@"; }
type_into() { # type_into <n> <text>: the n-th text field from the top (0 = top) gets <text>
  # shellcheck disable=SC2046 # "x y" are two arguments
  adb shell input tap $(screen field "$1")
  sleep 0.5
  adb shell input text "$2"
  adb shell input keyevent KEYCODE_BACK
  sleep 0.5
}
wait_for() { # wait_for: until the film, or one of the two certificate messages, is on the screen; leaves the texts in $out
  for _ in $(seq 1 20); do
    sleep 0.5
    out="$(screen texts)"
    if grep -qx 'Film Behind TLS' <<<"$out" || grep -q -e 'does not trust the server' -e 'is not the one it had' <<<"$out"; then return 0; fi
  done
}
login() { # a fresh app, the fake server's address and an account
  adb shell pm clear "$app" >/dev/null
  adb shell am start -n "$app/io.github.chicohaager.moviescast.MainActivity" >/dev/null
  out=""
  for _ in $(seq 1 20); do
    out="$(screen texts)"
    if grep -qx 'Log in' <<<"$out"; then break; fi
    sleep 0.5
  done
  if ! grep -qx 'Log in' <<<"$out"; then echo "the login page did not come (is the system language English?)" >&2; echo "$out" >&2; exit 1; fi
  : > "$tmp/server.log"
  type_into 0 "https://127.0.0.1:$port"
  type_into 1 demo
  type_into 2 right
  # shellcheck disable=SC2046 # "x y" are two arguments
  adb shell input tap $(screen tap 'Log in')
  wait_for
}
reopen() { # the app started again, its login kept
  adb shell am force-stop "$app"
  : > "$tmp/server.log"
  adb shell am start -n "$app/io.github.chicohaager.moviescast.MainActivity" >/dev/null
  wait_for
}
check() { # check <case> <expected: refused|trusted|changed>
  local logins requests shown
  logins="$(grep -c '^POST /movies/Users/AuthenticateByName$' "$tmp/server.log" || true)"
  requests="$(grep -c -E '^(GET|POST) ' "$tmp/server.log" || true)"
  if grep -q 'does not trust the server' <<<"$out"; then shown=refused
  elif grep -q 'is not the one it had' <<<"$out"; then shown=changed
  elif grep -qx 'Film Behind TLS' <<<"$out"; then shown=trusted
  else shown=neither; fi
  echo "$1: the app shows: $shown, requests at the server: $requests (logins $logins)"
  local good=0
  if [[ "$2" == refused && "$shown" == refused && "$requests" == 0 ]]; then good=1; fi
  if [[ "$2" == trusted && "$shown" == trusted && "$logins" == 1 ]]; then good=1; fi
  if [[ "$2" == changed && "$shown" == changed && "$requests" == 0 ]]; then good=1; fi
  if [[ "$good" != 1 ]]; then echo "FAIL $1, expected $2; the screen:" >&2; echo "$out" >&2; exit 1; fi
}

make_cert own
make_cert other
for s in "${stores[@]}"; do adb shell rm -f "$s"; done
serve own
adb reverse "tcp:$port" "tcp:$port" >/dev/null
adb install -r "$apk" >/dev/null

login
check "1 not installed" refused
install_cert own
login
check "2 own installed, log in" trusted
install_cert other
serve other
reopen
check "3 the server shows the other, also installed" changed
for s in "${stores[@]}"; do adb shell rm -f "$s"; done
serve own
login
check "4 both removed, log in" refused
echo "user certificates: only the one of the login counts ($(adb shell getprop ro.product.model | tr -d '\r'), Android $(adb shell getprop ro.build.version.release | tr -d '\r'))"
