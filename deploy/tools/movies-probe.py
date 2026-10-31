"""Ask a Movies server a few GET questions and print only named fields — log in, ask, log out, prove the logout.

    movies-probe.py --fields Id,Name,UserData.PlaybackPositionTicks /Users/{uid}/Items?Recursive=true [PATH ...]

Why: on 2026-10-28 I wrote 31 Movies logins by hand; 15 never logged out and 3 printed the access token.
This tool is the only way to query Movies from a session (memory project-movies-cast).

- Login like resume-guard.py: MOVIES_URL, MOVIES_USER, MOVIES_PASSWORD_FILE (the password is read from the file).
- `{uid}` in a path is replaced by the logged-in user's Id.
- Output is an allowlist: only the fields named in --fields (dotted for nested, e.g. UserData.Played). A response
  with "Items" prints one line per item; a list prints one line per element; an object prints one line.
- Fields whose name contains Url/Uri/Href/Link print as urlsplit().path only (no host, no query: query strings can
  carry api_key). Field names that look like secrets (token, password, pw, secret, key) are refused before login.
- The token is never printed: every output line is checked and the token replaced; errors print the HTTP status
  and the path, never the response body or the exception text.
- Logout always runs (finally). Afterwards the old token must answer 401, otherwise exit 3.

Exit: 0 ok · 1 a request failed · 2 usage/login failed · 3 token still valid after logout
"""
import argparse
import importlib.util
import json
import os
import pathlib
import re
import sys
import urllib.error
import urllib.parse

_spec = importlib.util.spec_from_file_location("resume_guard", pathlib.Path(__file__).resolve().parent / "resume-guard.py")
_rg = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_rg)
Movies = _rg.Movies

URL_NAME = re.compile(r"url|uri|href|link", re.I)
SECRET_NAME = re.compile(r"token|password|passwd|secret|key|^pw$|\.pw$", re.I)


class Out:
    """Every line goes through here; the token (and the password) are replaced before anything is printed."""

    def __init__(self):
        self.secrets = []

    def __call__(self, line, err=False):
        for s in self.secrets:
            if s:
                line = line.replace(s, "<redacted>")
        print(line, file=sys.stderr if err else sys.stdout, flush=True)


def pick(obj, dotted):
    cur = obj
    for part in dotted.split("."):
        if not isinstance(cur, dict) or part not in cur:
            return None, False
        cur = cur[part]
    return cur, True


def clean(name, value):
    """URL-named fields → path only, also inside nested values; other scalars as they are."""
    if isinstance(value, dict):
        return {k: clean(k, v) for k, v in value.items()}
    if isinstance(value, list):
        return [clean(name, v) for v in value]
    if isinstance(value, str) and URL_NAME.search(name):
        return urllib.parse.urlsplit(value).path
    return value


def line_for(obj, fields):
    parts = []
    for f in fields:
        v, found = pick(obj, f)
        if not found:
            parts.append(f"{f}=<missing>")
            continue
        v = clean(f.split(".")[-1], v)
        parts.append(f"{f}={json.dumps(v, ensure_ascii=False) if isinstance(v, (dict, list)) else v}")
    return "  ".join(parts)


def err_text(e):
    if isinstance(e, urllib.error.HTTPError):
        return f"HTTP {e.code}"
    return type(e).__name__


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--fields", required=True, help="comma-separated allowlist, dotted for nested fields")
    ap.add_argument("paths", nargs="+", help="GET paths, e.g. /System/Info or /Users/{uid}/Items?Recursive=true")
    a = ap.parse_args()
    fields = [f.strip() for f in a.fields.split(",") if f.strip()]
    bad = [f for f in fields if SECRET_NAME.search(f)]
    if not fields or bad:
        sys.exit(f"movies-probe: refused field name(s): {', '.join(bad) or '(none given)'}")
    for p in a.paths:
        if not p.startswith("/"):
            sys.exit(f"movies-probe: path must start with /: {p}")
    for k in ("MOVIES_URL", "MOVIES_USER", "MOVIES_PASSWORD_FILE"):
        if not os.environ.get(k):
            sys.exit(f"movies-probe: {k} is not set")

    out = Out()
    with open(os.environ["MOVIES_PASSWORD_FILE"]) as f:
        password = f.read().strip()
    out.secrets.append(password)
    try:
        m = Movies(os.environ["MOVIES_URL"], os.environ["MOVIES_USER"], password)
    except Exception as e:  # noqa: BLE001 — print the status only, never the exception text
        out(f"movies-probe: login failed: {err_text(e)}", err=True)
        return 2
    out.secrets.append(m.token)
    rc = 0
    try:
        for p in a.paths:
            path = p.replace("{uid}", urllib.parse.quote(m.uid))
            shown = urllib.parse.urlsplit(path).path
            try:
                res = m.call("GET", path)
            except Exception as e:  # noqa: BLE001
                out(f"{shown}: request failed: {err_text(e)}", err=True)
                rc = 1
                continue
            if isinstance(res, dict) and isinstance(res.get("Items"), list):
                rows = res["Items"]
                out(f"{shown}: {len(rows)} items" + (f" of {res['TotalRecordCount']}" if "TotalRecordCount" in res else ""))
            elif isinstance(res, list):
                rows = res
                out(f"{shown}: {len(rows)} elements")
            else:
                rows = [res]
                out(f"{shown}:")
            for row in rows:
                out("  " + (line_for(row, fields) if isinstance(row, dict) else "<not an object>"))
    finally:
        try:
            m.logout()
            out("logout: done")
        except Exception as e:  # noqa: BLE001
            out(f"logout: failed: {err_text(e)}", err=True)
            rc = rc or 1
        # the old token must be dead now
        try:
            m.call("GET", "/System/Info")
            out("logout: the token is still valid after logout", err=True)
            rc = 3
        except urllib.error.HTTPError as e:
            if e.code == 401:
                out("logout: token answers 401 afterwards")
            else:
                out(f"logout: token check answered HTTP {e.code}, not 401", err=True)
                rc = 3
        except Exception as e:  # noqa: BLE001
            out(f"logout: token check failed: {err_text(e)}", err=True)
            rc = 3
    return rc


if __name__ == "__main__":
    sys.exit(main())
