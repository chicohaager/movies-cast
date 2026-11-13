#!/usr/bin/env python3
"""Turns the icon masters (icon-*.svg) into Android VectorDrawables, so app and master never drift apart.

Knows only what the masters use and refuses anything else: <path> with d, fill / stroke as a colour or
url(#gradient), fill-opacity, stroke-opacity, stroke-width, stroke-linecap, stroke-linejoin; <g> with
transform="rotate(a cx cy)"; linear and radial gradients in userSpaceOnUse with stop-color / stop-opacity.

    python3 svg2vd.py icon-foreground.svg ../app/src/main/res/drawable/ic_launcher_foreground.xml
"""
import re
import sys
import xml.etree.ElementTree as ET

SVG = "{http://www.w3.org/2000/svg}"
PATH_ATTRS = {"d", "fill", "fill-opacity", "stroke", "stroke-width", "stroke-opacity", "stroke-linecap", "stroke-linejoin"}


def argb(color: str, opacity: str | None) -> str:
    color = color.strip()
    if not re.fullmatch(r"#[0-9A-Fa-f]{6}", color):
        raise SystemExit(f"colour not supported: {color}")
    a = round(float(opacity if opacity is not None else 1) * 255)
    return f"#{a:02X}{color[1:].upper()}"


def gradient_xml(g: ET.Element, indent: str) -> str:
    if g.get("gradientUnits") != "userSpaceOnUse":
        raise SystemExit(f"gradient {g.get('id')}: only userSpaceOnUse")
    kind = g.tag.removeprefix(SVG)
    if kind == "linearGradient":
        head = (f'android:type="linear" android:startX="{g.get("x1")}" android:startY="{g.get("y1")}" '
                f'android:endX="{g.get("x2")}" android:endY="{g.get("y2")}"')
    elif kind == "radialGradient":
        head = f'android:type="radial" android:centerX="{g.get("cx")}" android:centerY="{g.get("cy")}" android:gradientRadius="{g.get("r")}"'
    else:
        raise SystemExit(f"not a gradient: {kind}")
    items = [f'{indent}    <item android:offset="{s.get("offset")}" android:color="{argb(s.get("stop-color"), s.get("stop-opacity"))}" />'
             for s in g.findall(f"{SVG}stop")]
    return f"{indent}<gradient {head}>\n" + "\n".join(items) + f"\n{indent}</gradient>"


def paint(value: str, opacity: str | None, gradients: dict, attr: str, indent: str) -> tuple[list[str], list[str]]:
    """Returns (attributes, child elements) for android:fillColor / android:strokeColor."""
    m = re.fullmatch(r"url\(#([\w-]+)\)", value)
    if m:
        child = (f'{indent}    <aapt:attr name="android:{attr}">\n'
                 f"{gradient_xml(gradients[m.group(1)], indent + '        ')}\n"
                 f"{indent}    </aapt:attr>")
        alpha = [f'android:{"fillAlpha" if attr == "fillColor" else "strokeAlpha"}="{opacity}"'] if opacity else []
        return alpha, [child]
    return [f'android:{attr}="{argb(value, opacity)}"'], []


def path_xml(p: ET.Element, gradients: dict, indent: str) -> str:
    unknown = set(p.attrib) - PATH_ATTRS
    if unknown:
        raise SystemExit(f"path attribute not supported: {sorted(unknown)}")
    attrs = [f'android:pathData="{p.get("d")}"']
    children: list[str] = []
    fill = p.get("fill", "#000000")
    if fill != "none":
        a, c = paint(fill, p.get("fill-opacity"), gradients, "fillColor", indent)
        attrs += a; children += c
    stroke = p.get("stroke")
    if stroke and stroke != "none":
        a, c = paint(stroke, p.get("stroke-opacity"), gradients, "strokeColor", indent)
        attrs += a; children += c
        attrs.append(f'android:strokeWidth="{p.get("stroke-width", "1")}"')
        if p.get("stroke-linecap"):
            attrs.append(f'android:strokeLineCap="{p.get("stroke-linecap")}"')
        if p.get("stroke-linejoin"):
            attrs.append(f'android:strokeLineJoin="{p.get("stroke-linejoin")}"')
    head = f"{indent}<path\n" + "\n".join(f"{indent}    {a}" for a in attrs)
    return head + (">\n" + "\n".join(children) + f"\n{indent}</path>" if children else " />")


def node_xml(e: ET.Element, gradients: dict, indent: str, inherited: dict) -> list[str]:
    tag = e.tag.removeprefix(SVG)
    if tag == "path":
        merged = dict(inherited); merged.update(e.attrib)
        return [path_xml(ET.Element(e.tag, merged), gradients, indent)]
    if tag == "g":
        rest = {k: v for k, v in e.attrib.items() if k != "transform"}
        inner_inherited = dict(inherited); inner_inherited.update(rest)
        body = [x for c in e for x in node_xml(c, gradients, indent + "    ", inner_inherited)]
        t = e.get("transform")
        if not t:
            return [x for c in e for x in node_xml(c, gradients, indent, inner_inherited)]
        m = re.fullmatch(r"rotate\(([-\d.]+) ([-\d.]+) ([-\d.]+)\)", t)
        if not m:
            raise SystemExit(f"transform not supported: {t}")
        return [f'{indent}<group android:rotation="{m.group(1)}" android:pivotX="{m.group(2)}" android:pivotY="{m.group(3)}">'] + body + [f"{indent}</group>"]
    if tag == "defs":
        return []
    raise SystemExit(f"element not supported: {tag}")


def convert(svg_text: str, source: str) -> str:
    root = ET.fromstring(svg_text)
    x, y, w, h = root.get("viewBox").split()
    if (x, y) != ("0", "0"):
        raise SystemExit("viewBox must start at 0 0")
    gradients = {g.get("id"): g for g in root.iter() if g.tag in (f"{SVG}linearGradient", f"{SVG}radialGradient")}
    body = [x for c in root for x in node_xml(c, gradients, "    ", {})]
    return ('<?xml version="1.0" encoding="utf-8"?>\n'
            f"<!-- Written by android/art/svg2vd.py from android/art/{source}; change the master and run it again. -->\n"
            '<vector xmlns:android="http://schemas.android.com/apk/res/android"\n'
            '    xmlns:aapt="http://schemas.android.com/aapt"\n'
            f'    android:width="{w}dp"\n    android:height="{h}dp"\n'
            f'    android:viewportWidth="{w}"\n    android:viewportHeight="{h}">\n'
            + "\n".join(body) + "\n</vector>\n")


if __name__ == "__main__":
    src, out = sys.argv[1], sys.argv[2]
    with open(src) as f:
        text = convert(f.read(), src.rsplit("/", 1)[-1])
    with open(out, "w") as f:
        f.write(text)
    print(f"{out}: {text.count('<path')} paths, {text.count('<gradient')} gradients")
