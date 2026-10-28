"""Write one installation's id, vendor and server URL into a copy of the TV app.

    apply-local.py <app-copy-dir> <app-id> <vendor> <server-url>

Empty arguments keep the default. Called by package.sh, never on deploy/webos/app.
"""
import json
import sys

DEFAULT_SERVER = 'var SERVER = "http://nas.local:8097";'

app, app_id, vendor, server = sys.argv[1:]

p = app + "/appinfo.json"
with open(p) as f:
    info = json.load(f)
if app_id:
    info["id"] = app_id
if vendor:
    info["vendor"] = vendor
with open(p, "w") as f:
    json.dump(info, f, indent=2, ensure_ascii=False)

if server:
    p = app + "/index.html"
    with open(p) as f:
        s = f.read()
    if s.count(DEFAULT_SERVER) != 1:
        sys.exit("apply-local: SERVER line not found once in index.html")
    with open(p, "w") as f:
        f.write(s.replace(DEFAULT_SERVER, "var SERVER = " + json.dumps(server.rstrip("/")) + ";"))

sys.stdout.write(f"packaging {info['id']} {info['version']}\n")
