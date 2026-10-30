#!/usr/bin/env python3
"""Renders the launcher icon and the Android TV banner from the movies-cast logo.

  python3 deploy/androidtv/render.py   (Playwright/Chromium, like deploy/webos/logo/render.py)

res/mipmap-xxxhdpi/ic_launcher.png  192 px, the logo
res/drawable-xhdpi/banner.png       320x180, logo and name (the Android TV home row needs a banner)
"""
from pathlib import Path

from playwright.sync_api import sync_playwright

HERE = Path(__file__).resolve().parent
LOGO = (HERE.parent / "webos" / "logo" / "logo.svg").read_text()
ROOT = 'viewBox="0 0 512 512" width="512" height="512"'
assert LOGO.count(ROOT) == 1, "logo.svg: root size not found"


def sized(px):
    """The logo with its root element at px; the 512 inside stay viewBox units."""
    return LOGO.replace(ROOT, f'viewBox="0 0 512 512" width="{px}" height="{px}"')


BANNER = f"""<!doctype html><html><body style="margin:0">
<div style="width:320px;height:180px;display:flex;align-items:center;justify-content:center;
  background:linear-gradient(#1d2232,#0b0d12);font:bold 34px sans-serif;color:#eef0f5">
  <div style="width:104px;height:104px;margin-right:14px">{sized(104)}</div>
  <div>Movies<br>Cast</div>
</div></body></html>"""

ICON = f"""<!doctype html><html><body style="margin:0;background:transparent">
{sized(192)}</body></html>"""

with sync_playwright() as p:
    b = p.chromium.launch()
    for html, size, out in ((ICON, (192, 192), "res/mipmap-xxxhdpi/ic_launcher.png"),
                            (BANNER, (320, 180), "res/drawable-xhdpi/banner.png")):
        pg = b.new_page(viewport={"width": size[0], "height": size[1]})
        pg.set_content(html)
        pg.screenshot(path=str(HERE / out), omit_background=True)
        pg.close()
        print("wrote", out)
    b.close()
