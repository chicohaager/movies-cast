#!/usr/bin/env python3
"""strings-check.py <res-dir>: every values-*/strings.xml has exactly the translatable keys of
values/strings.xml, with the same placeholders. Exit 1 and one line per finding otherwise."""
import glob, os, re, sys
import xml.etree.ElementTree as ET

def load(path, all_keys=False):
    out = {}
    for e in ET.parse(path).getroot().findall("string"):
        if all_keys or e.get("translatable") != "false":
            out[e.get("name")] = e.text or ""
    return out

res = sys.argv[1]
base = load(os.path.join(res, "values", "strings.xml"))
files = sorted(glob.glob(os.path.join(res, "values-*", "strings.xml")))
bad = 0
for f in files:
    lang = os.path.basename(os.path.dirname(f))
    got = load(f, all_keys=True)
    for k in sorted(set(base) - set(got)):
        print(f"{lang}: {k} is missing"); bad += 1
    for k in sorted(set(got) - set(base)):
        print(f"{lang}: {k} is not a translatable key of values/strings.xml"); bad += 1
    for k in sorted(set(base) & set(got)):
        want = sorted(re.findall(r"%\d\$[sd]", base[k])); have = sorted(re.findall(r"%\d\$[sd]", got[k]))
        if want != have:
            print(f"{lang}: {k} has placeholders {have}, values/strings.xml has {want}"); bad += 1
        if not got[k].strip():
            print(f"{lang}: {k} is empty"); bad += 1
print(f"strings: {len(files)} translations, {len(base)} keys each, {bad} finding(s)")
sys.exit(1 if bad else 0)
