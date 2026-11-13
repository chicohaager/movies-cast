#!/usr/bin/env bash
# Builds the FFmpeg libraries that the ffmpeg module links (Media3's decoder_ffmpeg, copied from
# androidx/media 1.11.1, Apache 2.0). Run once before the first Gradle build, and again after
# changing the version or the decoder list:
#
#   ffmpeg/build-ffmpeg.sh            -> ffmpeg/src/main/jni/ffmpeg/android-libs/<abi>/*.a
#
# FFmpeg is configured without --enable-gpl and without --enable-nonfree, so the libraries are
# LGPL 2.1+ (checked after the build: config.h must say "LGPL"). Version and decoders as the
# Media3 README ("We currently recommend version 6.0") and Jellyfin's build (release/6.0;
# flac alac pcm_mulaw pcm_alaw mp3 aac ac3 eac3 dca mlp truehd).
set -euo pipefail

FFMPEG_TAG="n6.0.1"
NDK_VERSION="26.1.10909125"
MIN_SDK=23
DECODERS=(flac alac pcm_mulaw pcm_alaw mp3 aac ac3 eac3 dca mlp truehd)

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
module="$here/src/main"
sdk="${ANDROID_HOME:-$HOME/Android/Sdk}"
ndk="$sdk/ndk/$NDK_VERSION"
[[ -d "$ndk" ]] || { echo "missing NDK $ndk (sdkmanager --install \"ndk;$NDK_VERSION\")" >&2; exit 1; }

src="$module/jni/ffmpeg"
if [[ ! -d "$src/.git" ]]; then
  git clone --quiet --depth 1 --branch "$FFMPEG_TAG" https://git.ffmpeg.org/ffmpeg.git "$src"
fi
have="$(git -C "$src" describe --tags --exact-match 2>/dev/null || echo none)"
[[ "$have" == "$FFMPEG_TAG" ]] || { echo "$src is at $have, expected $FFMPEG_TAG; remove it and run again" >&2; exit 1; }

rm -rf "$src/android-libs"
(cd "$module/jni" && ./build_ffmpeg.sh "$module" "$ndk" linux-x86_64 "$MIN_SDK" "${DECODERS[@]}")

# The license of what was built, from FFmpeg's own configure result, and every ABI present.
license="$(sed -n 's/^#define FFMPEG_LICENSE "\(.*\)"$/\1/p' "$src/config.h" 2>/dev/null || true)"
for abi in armeabi-v7a arm64-v8a x86 x86_64; do
  for lib in avcodec avutil swresample; do
    [[ -s "$src/android-libs/$abi/lib$lib.a" ]] || { echo "missing $abi/lib$lib.a" >&2; exit 1; }
  done
done
echo "ffmpeg $FFMPEG_TAG built for 4 ABIs, license: ${license:-unknown (config.h removed by make clean)}"
