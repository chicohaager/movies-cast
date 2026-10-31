#!/usr/bin/env bash
# Package the webOS TV app with this installation's values.
#
#   deploy/webos/package.sh            -> deploy/webos/out/<id>_<version>_all.ipk
#
# deploy/webos/app holds neutral defaults. The values of one installation live
# in deploy/webos/local.env (git-ignored), for example:
#   APP_ID=com.example.moviescast     # keep it once installed: a new id is a second app
#   VENDOR="Your name"
#   SERVER_URL=http://nas.local:8097  # movies-cast as the TV reaches it
# Without local.env the defaults are packaged unchanged.
#
#   STORE=1 deploy/webos/package.sh    -> the build for the LG Content Store: it asks
#                                         for the server address (docs/WEBOS-STORE.md)
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
out="$here/out"
if [[ -f "$here/local.env" ]]; then
  # shellcheck disable=SC1091
  . "$here/local.env"
fi
rm -rf "${out:?}/app"
mkdir -p "$out"
cp -r "$here/app" "$out/app"
if [[ "${STORE:-}" == 1 ]]; then
  # The store build (roadmap 23): one app for everyone, asking for the address
  # of Movies-Cast (store-index.html) instead of a SERVER_URL built in.
  cp "$here/store-index.html" "$out/app/index.html"
  SERVER_URL=""
fi
python3 "$here/apply-local.py" "$out/app" "${APP_ID:-}" "${VENDOR:-}" "${SERVER_URL:-}"
ares-package "$out/app" -o "$out" -n
