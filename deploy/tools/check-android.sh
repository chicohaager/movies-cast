#!/usr/bin/env bash
# The Android app's part of ./check.sh: complete translations, unit tests, debug and release build, and
# properties of the APKs that no test sees: every translation is inside, the licences page names FFmpeg
# with the LGPL's text, the FFmpeg sound decoder is inside for every ABI, every native library is laid
# out for 16 KB pages (Android warns on such devices otherwise), and a release APK built with a key
# carries a valid signature.
#
#   deploy/tools/check-android.sh [apk]   with an APK: only the APK checks, no build
#   deploy/tools/check-android.sh --device [serial]
#                                           the tests on a device (app/src/androidTest): login,
#                                           library, film page, licences and diagnostics against a fake
#                                           server inside the test; a real login on the device is not touched
set -euo pipefail

repo="$(cd "$(dirname "$0")/../.." && pwd)"
apk="${1:-}"
device=0
if [[ "$apk" == "--device" ]]; then
  device=1; apk=""
  # Before anything is built: the one device adb has ready, or the one named (pick-device.sh says
  # which there are when it is none or several). Every adb call below then goes to that device.
  ANDROID_SERIAL="$("$repo/deploy/tools/pick-device.sh" ${2:+"$2"})"
  export ANDROID_SERIAL
fi
[[ -n "$apk" ]] && apk="$(realpath "$apk")"
cd "$repo/android"

if [[ "$device" == 1 ]]; then
  # Installed by hand and not with Gradle's connectedDebugAndroidTest, so the app and its data stay on the device.
  ./gradlew -q :app:assembleDebug :app:assembleDebugAndroidTest
  adb install -r app/build/outputs/apk/debug/app-debug.apk >/dev/null
  adb install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk >/dev/null
  # A run takes minutes: the Pixel 7 turned its screen off and locked after its 120 s mid-run, the Fire TV
  # stick started its screensaver (2026-11-13); every test after that failed with "No compose hierarchies
  # found". While the tests run the screen stays on while powered and the screensaver is off; the
  # device's own values come back afterwards, also when the run fails or is broken off.
  stay="$(adb shell settings get global stay_on_while_plugged_in | tr -d '\r')"
  saver="$(adb shell settings get secure screensaver_enabled | tr -d '\r')"
  restore_screen() {
    if [[ "$stay" == null ]]; then adb shell settings delete global stay_on_while_plugged_in >/dev/null || true
    else adb shell settings put global stay_on_while_plugged_in "$stay" || true; fi
    if [[ "$saver" == null ]]; then adb shell settings delete secure screensaver_enabled >/dev/null || true
    else adb shell settings put secure screensaver_enabled "$saver" || true; fi
  }
  trap restore_screen EXIT
  adb shell settings put global stay_on_while_plugged_in 7
  adb shell settings put secure screensaver_enabled 0
  adb shell input keyevent KEYCODE_WAKEUP
  adb shell wm dismiss-keyguard
  sleep 2
  # Behind the lock screen no activity starts and every test fails with "No compose hierarchies found"
  # (Pixel 7, 2026-11-13); a lock with a PIN cannot be opened from here.
  if adb shell dumpsys window | grep -q 'isKeyguardShowing=true'; then
    echo "the device is locked; unlock it and run this again" >&2
    exit 1
  fi
  # "am instrument" ends with 0 also when tests fail; its last lines say what happened.
  out="$(adb shell am instrument -w io.github.chicohaager.moviescast.debug.test/io.github.chicohaager.moviescast.TestRunner)"
  # The whole output stays, so a red run can be read afterwards even when only the last line was looked at.
  log="$repo/android/build/device-test-${ANDROID_SERIAL//[^A-Za-z0-9._-]/_}.log"
  mkdir -p "$(dirname "$log")"
  printf '%s\n' "$out" > "$log"
  if ! grep -qE '^OK \([0-9]+ tests?\)' <<<"$out"; then
    echo "$out" >&2
    echo "the tests on the device failed; the whole output: $log" >&2
    exit 1
  fi
  echo "android on $(adb shell getprop ro.product.model | tr -d '\r') (Android $(adb shell getprop ro.build.version.release | tr -d '\r')): $(grep -E '^OK \(' <<<"$out")"
  exit 0
fi

sdk="${ANDROID_HOME:-}"
if [[ -z "$sdk" && -f local.properties ]]; then
  sdk="$(sed -n 's/^sdk\.dir=//p' local.properties)"
fi
if [[ -z "$sdk" || ! -d "$sdk" ]]; then
  echo "no Android SDK (ANDROID_HOME or android/local.properties); run ./check.sh --no-android" >&2
  exit 1
fi
zipalign="$(ls "$sdk"/build-tools/*/zipalign | sort -V | tail -n 1)"

if [[ -z "$apk" ]]; then
  if [[ ! -d ffmpeg/src/main/jni/ffmpeg ]]; then
    echo "FFmpeg is not built; run android/ffmpeg/build-ffmpeg.sh once" >&2
    exit 1
  fi
  python3 tools/strings-check.py app/src/main/res
  # The release build shrinks code and resources; signed only where the key is (app/build.gradle.kts).
  # An APK of an earlier run with the other name would be checked in place of the new one.
  out=app/build/outputs/apk/release
  rm -f "$out/app-release.apk" "$out/app-release-unsigned.apk"
  # Results of an earlier run would be counted in with this one.
  rm -rf core/build/test-results app/build/test-results
  if ! ./gradlew -q :core:test :app:testDebugUnitTest :app:assembleDebug :app:assembleRelease; then
    # -q leaves only "N tests completed, M failed"; name them (and say so when no test ran)
    python3 tools/failed-tests.py core/build/test-results app/build/test-results >&2 || true
    exit 1
  fi
  release=""
  for f in "$out/app-release.apk" "$out/app-release-unsigned.apk"; do
    if [[ -f "$f" ]]; then release="$f"; fi
  done
  if [[ -z "$release" ]]; then
    echo "the release build left no APK in app/build/outputs/apk/release" >&2
    exit 1
  fi
  "$repo/deploy/tools/check-android.sh" app/build/outputs/apk/debug/app-debug.apk
  exec "$repo/deploy/tools/check-android.sh" "$release"
fi

# A translation that the build filters out never reaches a device (localeFilters in app/build.gradle.kts).
aapt2="$(dirname "$zipalign")/aapt2"
inside="$("$aapt2" dump configurations "$apk")"
for dir in app/src/main/res/values-*/; do
  [[ -f "$dir/strings.xml" ]] || continue
  lang="$(basename "$dir")"; lang="${lang#values-}"
  if ! grep -qx "$lang" <<<"$inside"; then
    echo "the translation $lang is not in $apk" >&2
    exit 1
  fi
done

listing="$(unzip -Z1 "$apk")"
for abi in arm64-v8a armeabi-v7a x86 x86_64; do
  if ! grep -qx "lib/$abi/libffmpegJNI.so" <<<"$listing"; then
    echo "libffmpegJNI.so for $abi is missing in $apk" >&2
    exit 1
  fi
done
# The launcher icon's vector layers are written from android/art/icon-*.svg by svg2vd.py; a hand edit of
# one side only would let the app's icon and the logo of the other apps drift apart.
art_tmp="$(mktemp -d)"
for layer in background foreground monochrome; do
  python3 "$repo/android/art/svg2vd.py" "$repo/android/art/icon-$layer.svg" "$art_tmp/$layer.xml" >/dev/null
  if ! cmp -s "$art_tmp/$layer.xml" "$repo/android/app/src/main/res/drawable/ic_launcher_$layer.xml"; then
    rm -rf "$art_tmp"
    echo "res/drawable/ic_launcher_$layer.xml is not what android/art/svg2vd.py makes of icon-$layer.svg (run android/art/render.sh)" >&2
    exit 1
  fi
done
rm -rf "$art_tmp"
# The licences page reads raw/aboutlibraries at run time. FFmpeg is no Gradle dependency, so only
# android/config/libraries/ffmpeg.json puts it there (without that file this check fails; the LGPL's
# text came from the plugin even without android/config/licenses, measured 2026-11-13).
# (Read into a variable first: a reader that stops early would end aapt2 with SIGPIPE under pipefail.)
resources="$("$aapt2" dump resources "$apk")"
licences="$(awk '/resource 0x[0-9a-f]+ raw\/aboutlibraries$/ { getline; print $NF; exit }' <<<"$resources")"
if [[ -z "$licences" ]]; then
  echo "raw/aboutlibraries (the licences page) is not in $apk" >&2
  exit 1
fi
licence_text="$(unzip -p "$apk" "$licences")"
# Space Grotesk (res/font, the display font) is no Gradle dependency either: android/config/libraries/space-grotesk.json.
for want in '"uniqueId": *"org.ffmpeg:ffmpeg"' 'GNU LESSER GENERAL PUBLIC LICENSE' 'Apache License' \
    '"uniqueId": *"com.github.floriankarsten:space-grotesk"' 'SIL OPEN FONT LICENSE'; do
  if ! grep -q -- "$want" <<<"$licence_text"; then
    echo "the licences file $licences in $apk lacks: $want" >&2
    exit 1
  fi
done
if ! "$zipalign" -c -P 16 4 "$apk" >/dev/null; then
  echo "$apk is not aligned for 16 KB pages (zipalign -c -P 16 4)" >&2
  exit 1
fi
# zipalign looks at the place in the APK; the segments inside the library need 16 KB as well.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
for abi in arm64-v8a x86_64; do
  unzip -q -o "$apk" "lib/$abi/*.so" -d "$tmp"
  for so in "$tmp/lib/$abi"/*.so; do
    for align in $(readelf -lW "$so" | awk '$1 == "LOAD" { print $NF }'); do
      if (( align < 16384 )); then
        echo "$abi/$(basename "$so") has a LOAD segment aligned to $align, below 16 KB" >&2
        exit 1
      fi
    done
  done
done
signed=""
if [[ "$(basename "$apk")" == app-release.apk ]]; then
  if ! "$(dirname "$zipalign")/apksigner" verify "$apk" >/dev/null 2>&1; then
    echo "$apk has no valid signature (apksigner verify)" >&2
    exit 1
  fi
  signed=", signature valid"
fi
echo "android $(basename "$apk"): translations in the APK$signed, FFmpeg and the LGPL, Space Grotesk and the OFL on the licences page, icon layers as drawn in android/art, libffmpegJNI.so in 4 ABIs, 16 KB alignment in the APK and in the 64-bit libraries ok"
