#!/usr/bin/env python3
"""Builds the movies-cast logo (a 512 rounded square: the sky of icon-background.svg, the mark of
icon-foreground.svg) and puts it where the other apps take it from:

  deploy/webos/logo/logo.svg          source of the webOS icons and the phone page's icons (render.py there)
  internal/server/web/logo.svg        the copy the server serves (TestLogoServedIsTheIconSource)
  deploy/androidtv/assets/logo.svg    the copy deploy/androidtv/build.sh compares
  deploy/zimaos/docker-compose.yml    the ZimaOS tile: both icon: lines as a data: URI (TestComposeIconIsTheLogo)

    python3 android/art/make-logo.py
"""
import base64
import pathlib
import re

ART = pathlib.Path(__file__).resolve().parent
REPO = ART.parent.parent
# The part of the 108 canvas the logo shows: 14…94 (the launcher shows 18…90 of an adaptive icon; a little
# more sky around the mark here, as nothing masks it).
WINDOW = (14, 80)


def inner(name: str) -> str:
    text = (ART / name).read_text()
    body = re.search(r"<svg[^>]*>(.*)</svg>", text, re.DOTALL)
    assert body, name
    return body.group(1).strip()


def logo() -> str:
    start, size = WINDOW
    scale = 512 / size
    return (
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">\n'
        "  <!-- movies-cast logo: a play sign as a planet with an orbit and a sending satellite, in deep space.\n"
        "       Written by android/art/make-logo.py from android/art/icon-background.svg and icon-foreground.svg. -->\n"
        '  <defs><clipPath id="corner"><rect width="512" height="512" rx="112"/></clipPath></defs>\n'
        '  <g clip-path="url(#corner)">\n'
        f'    <g transform="scale({scale:g}) translate({-start} {-start})">\n'
        f"{inner('icon-background.svg')}\n{inner('icon-foreground.svg')}\n"
        "    </g>\n  </g>\n</svg>\n"
    )


def main() -> None:
    svg = logo()
    for path in ("deploy/webos/logo/logo.svg", "internal/server/web/logo.svg", "deploy/androidtv/assets/logo.svg"):
        (REPO / path).write_text(svg)
        print("wrote", path)
    compose = REPO / "deploy/zimaos/docker-compose.yml"
    text = compose.read_text()
    uri = "data:image/svg+xml;base64," + base64.b64encode(svg.encode()).decode()
    lines = re.findall(r"^\s*icon: data:image/svg\+xml;base64,[A-Za-z0-9+/=]+$", text, re.M)
    assert len(lines) == 2, f"compose: {len(lines)} icon lines, want 2"
    text = re.sub(r"(^\s*icon: )data:image/svg\+xml;base64,[A-Za-z0-9+/=]+$", lambda m: m.group(1) + uri, text, flags=re.M)
    compose.write_text(text)
    print("wrote deploy/zimaos/docker-compose.yml (2 icons)")


if __name__ == "__main__":
    main()
