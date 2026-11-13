#!/usr/bin/env bash
# The Android app's part of ./check.sh: complete translations, unit tests, debug and release build, and
# properties of the APKs that no test sees: every translation is inside, the FFmpeg sound decoder is
# inside for every ABI, every native library is laid out for 16 KB pages (Android warns on such
# devices otherwise), and a release APK built with a key carries a valid signature.
#
#   deploy/tools/check-android.sh [apk]   with an APK: only the APK checks, no build
set -euo pipefail

repo="$(cd "$(dirname "$0")/../.." && pwd)"
apk="${1:-}"
[[ -n "$apk" ]] && apk="$(realpath "$apk")"
cd "$repo/android"

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
  ./gradlew -q :core:test :app:testDebugUnitTest :app:assembleDebug :app:assembleRelease
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
echo "android $(basename "$apk"): translations in the APK$signed, libffmpegJNI.so in 4 ABIs, 16 KB alignment in the APK and in the 64-bit libraries ok"
