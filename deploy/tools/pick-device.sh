#!/usr/bin/env bash
# Which device do the screen tests run on? Prints its serial number.
#
#   deploy/tools/pick-device.sh [serial]
#
# With a serial (or ANDROID_SERIAL): that device, if adb lists it as ready. Without: the one device
# adb lists as ready. No device, a device that is not ready (unauthorized, offline) or more than one
# to choose from: exit 1 with a line that says so and names the serials, so nobody has to find out
# from adb's "more than one device/emulator".
set -euo pipefail

want="${1:-${ANDROID_SERIAL:-}}"
listed="$(adb devices | sed '1d' | tr -d '\r' | awk 'NF >= 2 { print $1, $2 }')"
ready="$(awk '$2 == "device" { print $1 }' <<<"$listed")"
count="$(grep -c . <<<"$ready" || true)"

if [[ -n "$want" ]]; then
  if grep -qxF -- "$want" <<<"$ready"; then
    echo "$want"
    exit 0
  fi
  state="$(awk -v s="$want" '$1 == s { print $2 }' <<<"$listed")"
  if [[ -n "$state" ]]; then
    echo "device $want is not ready (adb says: $state)" >&2
  else
    echo "adb does not list a device $want; ready: ${ready//$'\n'/, }" >&2
  fi
  exit 1
fi
if [[ "$count" == 0 ]]; then
  if [[ -n "$listed" ]]; then
    echo "no device is ready; adb lists: ${listed//$'\n'/, }" >&2
  else
    echo "adb lists no device" >&2
  fi
  exit 1
fi
if [[ "$count" != 1 ]]; then
  echo "$count devices are ready: ${ready//$'\n'/, } - name one: check-android.sh --device <serial> (or ANDROID_SERIAL)" >&2
  exit 1
fi
echo "$ready"
