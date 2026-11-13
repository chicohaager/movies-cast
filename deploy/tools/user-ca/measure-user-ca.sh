#!/usr/bin/env bash
# Does the app trust a certificate the person installed in Android's settings, and only then?
#
#   deploy/tools/user-ca/measure-user-ca.sh [serial]
#
# Needs an emulator with root (an image "Google APIs", not "Google Play"; android/README.md) whose
# system language is English, and the debug build (cd android && ./gradlew :app:assembleDebug).
# Makes a self-signed certificate for 127.0.0.1, starts fake-https.py with it on this machine and
# lets the device reach it as 127.0.0.1 (adb reverse), then logs in from the app's login page three
# times: before the certificate is in the user store, after it was put there as the settings put it,
# and after it was taken out again. The app's data on the device is cleared each time and at the end.
# Expected: the certificate message and no login at the server; the library with the fake film and
# one login; the message again.
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
store=""
cleanup() {
  if [[ -n "$server" ]]; then kill "$server" 2>/dev/null || true; fi
  if [[ -n "$store" ]]; then adb shell rm -f "$store" || true; fi
  adb reverse --remove "tcp:$port" 2>/dev/null || true
  adb shell pm clear "$app" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

openssl req -x509 -newkey rsa:2048 -nodes -keyout "$tmp/server.key" -out "$tmp/cert.pem" -days 2 \
  -subj "/CN=invented test server" -addext "subjectAltName=IP:127.0.0.1" \
  -addext "basicConstraints=critical,CA:TRUE" 2>"$tmp/openssl.log"
# Android names a CA file in its user store by the old subject hash and ".0" (what the settings write).
store="/data/misc/user/0/cacerts-added/$(openssl x509 -in "$tmp/cert.pem" -noout -subject_hash_old).0"
/usr/bin/python3 "$here/fake-https.py" "$tmp/cert.pem" "$tmp/server.key" "$port" 2>"$tmp/server.log" &
server=$!
for _ in $(seq 1 20); do
  if ss -ltn | grep -q ":$port "; then break; fi
  sleep 0.2
done
if ! ss -ltn | grep -q ":$port "; then echo "the fake server did not start:" >&2; cat "$tmp/server.log" >&2; exit 1; fi
adb reverse "tcp:$port" "tcp:$port" >/dev/null
adb install -r "$apk" >/dev/null

screen() { python3 "$repo/deploy/tools/android-screen.py" "$@"; }
type_into() { # type_into <n> <text>: the n-th text field from the top (0 = top) gets <text>
  # shellcheck disable=SC2046 # "x y" are two arguments
  adb shell input tap $(screen field "$1")
  sleep 0.5
  adb shell input text "$2"
  adb shell input keyevent KEYCODE_BACK
  sleep 0.5
}
login() { # a fresh app, the fake server's address and an account; leaves the screen's texts in $out
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
  for _ in $(seq 1 20); do
    sleep 0.5
    out="$(screen texts)"
    if grep -qx 'Film Behind TLS' <<<"$out" || grep -q 'does not trust the server' <<<"$out"; then break; fi
  done
}
check() { # check <case> <expected: refused|trusted>
  local saw shown
  saw="$(grep -c '^POST /movies/Users/AuthenticateByName$' "$tmp/server.log" || true)"
  if grep -q 'does not trust the server' <<<"$out"; then shown=refused
  elif grep -qx 'Film Behind TLS' <<<"$out"; then shown=trusted
  else shown=neither; fi
  echo "$1: the app shows: $shown, logins at the server: $saw"
  local good=0
  if [[ "$2" == refused && "$shown" == refused && "$saw" == 0 ]]; then good=1; fi
  if [[ "$2" == trusted && "$shown" == trusted && "$saw" == 1 ]]; then good=1; fi
  if [[ "$good" != 1 ]]; then echo "FAIL $1, expected $2; the screen:" >&2; echo "$out" >&2; exit 1; fi
}

adb shell rm -f "$store"
login
check "1 not installed" refused
adb push "$tmp/cert.pem" /data/local/tmp/mc-ca.pem >/dev/null
adb shell "mkdir -p /data/misc/user/0/cacerts-added && cp /data/local/tmp/mc-ca.pem $store && rm /data/local/tmp/mc-ca.pem && chown system:system $store && chmod 644 $store && restorecon $store"
login
check "2 installed" trusted
adb shell rm -f "$store"
login
check "3 removed again" refused
echo "user certificate: trusted once installed, refused without ($(adb shell getprop ro.product.model | tr -d '\r'), Android $(adb shell getprop ro.build.version.release | tr -d '\r'))"
