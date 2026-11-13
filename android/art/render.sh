#!/usr/bin/env bash
# Renders the app's pictures from the masters in this folder with headless Chrome (gradients, the font):
#   banner.html      -> res/drawable-{xhdpi,xxhdpi,xxxhdpi}/banner.png   (Android TV launcher, 320×180 dp)
#   icon-legacy.html -> res/mipmap-{m,h,xh,xxh,xxxh}dpi/ic_launcher.png  (Android 7; 8+ uses the vector layers)
#   icon-*.svg       -> res/drawable/ic_launcher_{background,foreground,monochrome}.xml (svg2vd.py)
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
res="$here/../app/src/main/res"
chrome="${CHROME:-google-chrome}"
shot() { # page scale width height out
  local tmp; tmp="$(mktemp -d)"
  "$chrome" --headless=new --disable-gpu --hide-scrollbars --allow-file-access-from-files \
    --default-background-color=00000000 --force-device-scale-factor="$2" --window-size="$3,$4" \
    --screenshot="$tmp/s.png" "file://$here/$1" >/dev/null 2>&1
  mv -f "$tmp/s.png" "$5"; rmdir "$tmp"
  echo "$5: $(python3 -c "import struct,sys;d=open(sys.argv[1],'rb').read(24);print('%dx%d'%struct.unpack('>II',d[16:24]))" "$5")"
}
for layer in background foreground monochrome; do
  python3 "$here/svg2vd.py" "$here/icon-$layer.svg" "$res/drawable/ic_launcher_$layer.xml"; done
for d in "xhdpi 1" "xxhdpi 1.5" "xxxhdpi 2"; do set -- $d
  mkdir -p "$res/drawable-$1"; shot banner.html "$2" 320 180 "$res/drawable-$1/banner.png"; done
for d in "mdpi 1" "hdpi 1.5" "xhdpi 2" "xxhdpi 3" "xxxhdpi 4"; do set -- $d
  shot icon-legacy.html "$2" 48 48 "$res/mipmap-$1/ic_launcher.png"; done
