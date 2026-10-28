"""Render logo.svg to the PNG sizes the webOS app needs (and a large copy).

    python3 render.py   (needs Playwright with Chromium)
"""
import pathlib
import sys

from playwright.sync_api import sync_playwright

here = pathlib.Path(__file__).resolve().parent
svg = (here / "logo.svg").read_text()
targets = {here.parent / "app" / "icon.png": 80, here.parent / "app" / "largeIcon.png": 130, here / "logo-512.png": 512}
if len(sys.argv) > 1:
    targets = {pathlib.Path(sys.argv[1]): int(sys.argv[2])}

with sync_playwright() as p:
    b = p.chromium.launch()
    for out, size in targets.items():
        pg = b.new_page(viewport={"width": size, "height": size})
        pg.set_content(f'<html><body style="margin:0;background:transparent">'
                       f'<div style="width:{size}px;height:{size}px">{svg.replace("<svg ", f"<svg style=\"width:{size}px;height:{size}px\" ", 1)}</div></body></html>')
        pg.screenshot(path=str(out), omit_background=True)
        print(out.name, size)
    b.close()
