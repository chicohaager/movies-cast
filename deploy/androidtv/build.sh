#!/usr/bin/env bash
# Builds the Android TV app (APK) with the SDK's own tools, no Gradle:
#   deploy/androidtv/build.sh 0.26.0      -> deploy/androidtv/out/movies-cast-tv-0.26.0.apk
#
# ANDROID_HOME   SDK (default ~/Android/Sdk) with build-tools and platforms/android-36
# KEYSTORE       signing key (default ~/.local/share/movies-cast/androidtv-release.jks);
# KEYSTORE_PASS  file holding its password (default: KEYSTORE with .pass).
#                Created on the first build. Keep both: an update must be signed with
#                the same key, or Android refuses to install it over the old app.
set -euo pipefail

VERSION="${1:?usage: build.sh <version, e.g. 0.26.0>}"
[[ "$VERSION" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || { echo "version must be N.N.N, got $VERSION" >&2; exit 2; }
CODE=$(( BASH_REMATCH[1] * 10000 + BASH_REMATCH[2] * 100 + BASH_REMATCH[3] ))

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SDK="${ANDROID_HOME:-$HOME/Android/Sdk}"
BT="$SDK/build-tools/$(ls "$SDK/build-tools" | sort -V | tail -n 1)"
JAR="$SDK/platforms/android-36/android.jar"
[ -f "$JAR" ] || { echo "missing $JAR" >&2; exit 1; }
KEYSTORE="${KEYSTORE:-$HOME/.local/share/movies-cast/androidtv-release.jks}"
KEYSTORE_PASS="${KEYSTORE_PASS:-${KEYSTORE%.jks}.pass}"

# The start page shows the same logo as the server pages.
cmp -s "$HERE/assets/logo.svg" "$HERE/../../internal/server/web/logo.svg" \
  || { echo "assets/logo.svg differs from internal/server/web/logo.svg" >&2; exit 1; }

if [ ! -f "$KEYSTORE" ]; then
  echo "creating signing key $KEYSTORE (keep it and $KEYSTORE_PASS for every later build)"
  ( umask 077
    mkdir -p "$(dirname "$KEYSTORE")"
    head -c 24 /dev/urandom | base64 | tr -d '/+=' > "$KEYSTORE_PASS"
    keytool -genkeypair -keystore "$KEYSTORE" -storepass:file "$KEYSTORE_PASS" -keypass:file "$KEYSTORE_PASS" \
      -alias moviescast -keyalg RSA -keysize 3072 -validity 10000 -dname "CN=Movies-Cast" >/dev/null )
fi

W="$(mktemp -d)"
trap 'rm -rf "$W"' EXIT
mkdir -p "$W/gen" "$W/classes" "$HERE/out"

"$BT/aapt2" compile --dir "$HERE/res" -o "$W/res.zip"
"$BT/aapt2" link -o "$W/app.apk" -I "$JAR" --manifest "$HERE/AndroidManifest.xml" -A "$HERE/assets" \
  --java "$W/gen" --min-sdk-version 23 --target-sdk-version 36 \
  --version-code "$CODE" --version-name "$VERSION" "$W/res.zip"
javac -source 11 -target 11 -Xlint:-options -encoding UTF-8 -classpath "$JAR" -d "$W/classes" \
  $(find "$HERE/src" "$W/gen" -name '*.java')
"$BT/d8" --release --min-api 23 --lib "$JAR" --output "$W" $(find "$W/classes" -name '*.class')
( cd "$W" && zip -q -X app.apk classes.dex )
"$BT/zipalign" -f -p 4 "$W/app.apk" "$W/aligned.apk"
OUT="$HERE/out/movies-cast-tv-$VERSION.apk"
"$BT/apksigner" sign --ks "$KEYSTORE" --ks-pass "file:$KEYSTORE_PASS" --ks-key-alias moviescast --out "$OUT" "$W/aligned.apk"
"$BT/apksigner" verify "$OUT"
echo "built $OUT (versionCode $CODE)"
