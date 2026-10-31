"""Tests for deploy/tools/movies-probe.py against a local fake Movies server (made-up values only).

    python3 deploy/tools/test_movies_probe.py      exit 1 on any failure

Cases: allowlisted fields only; URL-like fields as path only; the token never appears (not in output, not in
errors, not when the server echoes it in an error body); logout runs on success and on error; the tool checks that
the token answers 401 after logout and fails if it does not; token-like field names are refused before any login.
"""
import http.server
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import threading

TOOL = pathlib.Path(__file__).resolve().parent / "movies-probe.py"
TOKEN = "faketoken0123456789abcdef0123456789"  # made up
PASSWORD = "fake-password-not-real"            # made up


class State:
    def __init__(self, invalidate=True):
        self.valid = set()
        self.logouts = 0
        self.logins = 0
        self.invalidate = invalidate


def handler_for(st):
    class H(http.server.BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def token(self):
            h = self.headers.get("X-Emby-Authorization", "")
            return h.split('Token="', 1)[1].split('"', 1)[0] if 'Token="' in h else ""

        def send(self, code, obj=None):
            raw = json.dumps(obj).encode() if obj is not None else b""
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def do_POST(self):
            n = int(self.headers.get("Content-Length") or 0)
            body = json.loads(self.rfile.read(n) or b"null")
            if self.path == "/Users/AuthenticateByName":
                st.logins += 1
                if body.get("Pw") != PASSWORD:
                    return self.send(401, {"error": "bad"})
                st.valid.add(TOKEN)
                return self.send(200, {"AccessToken": TOKEN, "User": {"Id": "USERID0000", "Name": "testuser"}})
            if self.path == "/Sessions/Logout":
                if self.token() not in st.valid:
                    return self.send(401)
                st.logouts += 1
                if st.invalidate:
                    st.valid.discard(self.token())
                return self.send(204)
            self.send(404)

        def do_GET(self):
            if self.token() not in st.valid:
                return self.send(401)
            if self.path.startswith("/Users/USERID0000/Items"):
                return self.send(200, {"TotalRecordCount": 2, "Items": [
                    {"Id": "a1", "Name": "Film One", "Path": "/srv/media/one.mkv", "ProductionYear": 1999,
                     "ImageUrl": f"http://198.51.100.7/Items/a1/Images/Primary?api_key={TOKEN}",
                     "UserData": {"PlaybackPositionTicks": 600000000, "Played": False}},
                    {"Id": "a2", "Name": "Film Two", "Path": "/srv/media/two.mkv", "ProductionYear": 2004,
                     "ImageUrl": "http://198.51.100.7/Items/a2/Images/Primary",
                     "UserData": {"PlaybackPositionTicks": 0, "Played": True}}]})
            if self.path == "/System/Info":
                return self.send(200, {"ServerName": "fake", "Version": "0.0.1", "LocalAddress": f"http://198.51.100.7:8096/?t={TOKEN}"})
            if self.path == "/Boom":
                return self.send(500, {"error": f"internal, token {TOKEN}"})
            self.send(404)
    return H


fails = []


def check(name, ok, r=None):
    print(f"  {'ok  ' if ok else 'FAIL'} {name}")
    if not ok:
        fails.append(name)
        if r is not None:
            print("       rc=%s\n       %s" % (r.returncode, (r.stdout + r.stderr).strip().replace("\n", "\n       ")))


def run(args, invalidate=True):
    st = State(invalidate)
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler_for(st))
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    with tempfile.TemporaryDirectory() as t:
        pw = os.path.join(t, "pw")
        with open(pw, "w") as f:
            f.write(PASSWORD + "\n")
        env = {**os.environ, "MOVIES_URL": f"http://127.0.0.1:{srv.server_address[1]}", "MOVIES_USER": "testuser",
               "MOVIES_PASSWORD_FILE": pw}
        r = subprocess.run([sys.executable, str(TOOL), *args], capture_output=True, text=True, env=env, timeout=30,
                           stdin=subprocess.DEVNULL)
    srv.shutdown()
    r.st = st
    r.all = r.stdout + r.stderr
    return r


r = run(["--fields", "Id,Name,ImageUrl,UserData.PlaybackPositionTicks", "/Users/{uid}/Items?Recursive=true"])
check("items: rc 0, both films, nested field", r.returncode == 0 and "Film One" in r.stdout and "Film Two" in r.stdout
      and "600000000" in r.stdout, r)
check("items: unlisted fields (Path, ProductionYear, Played) not printed",
      "/srv/media" not in r.all and "1999" not in r.all and "Played" not in r.all, r)
check("items: ImageUrl as path only", "/Items/a1/Images/Primary" in r.stdout and "198.51.100.7" not in r.all
      and "api_key" not in r.all, r)
check("items: token nowhere in output", TOKEN not in r.all and PASSWORD not in r.all, r)
check("items: logged out once, token answered 401 afterwards", r.st.logouts == 1 and "401" in r.stdout, r)

r = run(["--fields", "ServerName,LocalAddress", "/System/Info"])
check("object: LocalAddress (…Address is not URL-named) — token scrubbed anyway", r.returncode == 0 and TOKEN not in r.all
      and "fake" in r.stdout, r)

r = run(["--fields", "Name", "/Boom"])
check("server error echoing the token: rc != 0, token not printed, still logged out",
      r.returncode != 0 and TOKEN not in r.all and r.st.logouts == 1 and "500" in r.all, r)

r = run(["--fields", "Name", "/Users/{uid}/Items"], invalidate=False)
check("logout does not invalidate: rc != 0, says the token is still valid", r.returncode != 0 and "still" in r.all
      and TOKEN not in r.all, r)

r = run(["--fields", "Name,AccessToken", "/System/Info"])
check("token-like field name refused before login", r.returncode != 0 and r.st.logins == 0, r)

r = run(["/System/Info"])
check("--fields is required (no login without it)", r.returncode != 0 and r.st.logins == 0, r)

r = run(["--fields", " , ", "/System/Info"])
check("empty --fields refused (no login)", r.returncode != 0 and r.st.logins == 0, r)

print(f"{len(fails)} failed")
sys.exit(1 if fails else 0)
