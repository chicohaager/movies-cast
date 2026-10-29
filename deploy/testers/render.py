"""Write the testers' package files from the repo's ZimaOS compose and the guides.

    render.py <repo> <out-dir> <version> <image>

The compose is deploy/zimaos/docker-compose.yml with the image pointing at the
public registry and RENDERERS left empty (each tester's TVs are found by SSDP or
added in "Manage TVs"). Called by make-package.sh.
"""
import os
import sys

import yaml

repo, out, version, image = sys.argv[1:]
here = os.path.dirname(os.path.abspath(__file__))

src = os.path.join(repo, "deploy", "zimaos", "docker-compose.yml")
with open(src) as f:
    compose = f.read()
local_image = "    image: 127.0.0.1:5000/movies-cast:local\n"
if compose.count(local_image) != 1:
    sys.exit(f"render: image line not found once in {src}")
compose = compose.replace(local_image, f"    image: {image}\n")
if compose.count("      - RENDERERS=\n") != 1:
    sys.exit("render: RENDERERS is not empty in the compose")

# The file must stay what ZimaOS parses: one service, port_map a string.
d = yaml.safe_load(compose)
svc = d["services"]["movies-cast"]
assert svc["image"] == image, svc["image"]
assert isinstance(d["x-casaos"]["port_map"], str)
assert d["x-casaos"]["icon"].startswith("data:image/svg+xml;base64,")

header = (
    f"# movies-cast {version} for ZimaOS: Dashboard -> \"+\" next to Apps -> custom app -> Import.\n"
    "# Guide: ANLEITUNG.de.html / GUIDE.en.html in the same package.\n"
)
with open(os.path.join(out, "docker-compose.yml"), "w") as f:
    f.write(header + compose)

for name in ("ANLEITUNG.de.html", "GUIDE.en.html"):
    with open(os.path.join(here, name)) as f:
        page = f.read()
    for key, value in (("{{VERSION}}", version), ("{{IMAGE}}", image)):
        if key not in page:
            sys.exit(f"render: {key} missing in {name}")
        page = page.replace(key, value)
    if "{{" in page:
        sys.exit(f"render: unfilled placeholder left in {name}")
    with open(os.path.join(out, name), "w") as f:
        f.write(page)

print(f"render: {out} ({version}, {image})")
