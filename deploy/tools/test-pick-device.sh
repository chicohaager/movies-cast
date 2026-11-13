#!/usr/bin/env bash
# Tests for pick-device.sh with an adb that prints a list made here (invented serials).
# Run by check.sh; each case prints "ok" or "FAIL" and the script exits 1 on the first failure.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/adb" <<'ADB'
#!/usr/bin/env bash
[[ "$1" == devices ]] || { echo "unexpected: adb $*" >&2; exit 9; }
printf 'List of devices attached\n'
cat "$ADB_LIST"
printf '\n'
ADB
chmod +x "$tmp/adb"

run() { # run <list> [serial]: sets rc, out (stdout) and err (stderr); ANDROID_SERIAL as the caller set it in $serial_env
  printf '%b' "$1" > "$tmp/list"
  set +e
  out="$(PATH="$tmp:$PATH" ADB_LIST="$tmp/list" ANDROID_SERIAL="${serial_env:-}" "$here/pick-device.sh" ${2:+"$2"} 2>"$tmp/err")"; rc=$?
  set -e
  err="$(cat "$tmp/err")"
}
fail() { echo "FAIL $1"; echo "rc=$rc out=$out err=$err"; exit 1; }
phone='PHONE0001\tdevice\n'
emu='emulator-5554\tdevice\n'

run "$phone"
[[ $rc == 0 && "$out" == PHONE0001 && -z "$err" ]] || fail "one device"
echo "ok 1 one device is taken"

run "$phone$emu"
[[ $rc == 1 && -z "$out" && "$err" == "2 devices are ready: PHONE0001, emulator-5554 - name one:"* ]] || fail "two devices"
echo "ok 2 two devices: stops and names both"

run "$phone$emu" emulator-5554
[[ $rc == 0 && "$out" == emulator-5554 ]] || fail "two devices, one named"
echo "ok 3 the named one of two is taken"

serial_env=PHONE0001 run "$phone$emu"
[[ $rc == 0 && "$out" == PHONE0001 ]] || fail "ANDROID_SERIAL"
echo "ok 4 ANDROID_SERIAL names the device"

serial_env=PHONE0001 run "$phone$emu" emulator-5554
[[ $rc == 0 && "$out" == emulator-5554 ]] || fail "argument before ANDROID_SERIAL"
echo "ok 5 the argument goes before ANDROID_SERIAL"

run ""
[[ $rc == 1 && -z "$out" && "$err" == "adb lists no device" ]] || fail "no device"
echo "ok 6 no device"

run 'PHONE0001\tunauthorized\n'
[[ $rc == 1 && -z "$out" && "$err" == "no device is ready; adb lists: PHONE0001 unauthorized" ]] || fail "unauthorized"
echo "ok 7 a device that is not ready is not taken"

run "$phone" OTHER0002
[[ $rc == 1 && -z "$out" && "$err" == "adb does not list a device OTHER0002; ready: PHONE0001" ]] || fail "unknown serial"
echo "ok 8 a serial adb does not list"

run 'PHONE0001\toffline\n'"$emu" PHONE0001
[[ $rc == 1 && -z "$out" && "$err" == "device PHONE0001 is not ready (adb says: offline)" ]] || fail "named but offline"
echo "ok 9 the named device is offline"

# a serial that is the beginning of another one is not that one
run 'emulator-55540\tdevice\n' emulator-5554
[[ $rc == 1 && -z "$out" ]] || fail "prefix of a serial"
echo "ok 10 the beginning of a serial is not the serial"
