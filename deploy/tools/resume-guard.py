"""Save and restore the resume points of a Movies user around a test.

A test playback is a real playback: every stop reports its position, and
Movies keeps it as the user's resume point on every device (2026-10-28 a test
on the TV overwrote a real one). So a test on a real library runs between

    resume-guard.py save    <state.json>
    resume-guard.py restore <state.json>

restore writes back every position that changed and exits 1 if Movies does not
answer with the saved state afterwards.

Environment: MOVIES_URL (e.g. http://nas.local/movies), MOVIES_USER,
MOVIES_PASSWORD_FILE (file holding the password). Prints titles and positions,
never the password or the token.
"""
import json
import os
import sys
import urllib.parse
import urllib.request
import uuid

TICKS = 10_000_000  # Movies counts positions in 100 ns ticks, like Jellyfin


class Movies:
    def __init__(self, base, user, password):
        self.base = base.rstrip("/")
        self.device = "resume-guard-" + uuid.uuid4().hex[:8]
        self.token = ""
        res = self.call("POST", "/Users/AuthenticateByName", {"Username": user, "Pw": password})
        self.token, self.uid = res["AccessToken"], res["User"]["Id"]

    def call(self, method, path, body=None):
        auth = f'MediaBrowser Client="resume-guard", Device="resume-guard", DeviceId="{self.device}", Version="1"'
        if self.token:
            auth += f', Token="{self.token}"'
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(self.base + path, data=data, method=method,
                                     headers={"X-Emby-Authorization": auth, "Content-Type": "application/json"})
        with urllib.request.urlopen(req, timeout=15) as r:
            raw = r.read()
        return json.loads(raw) if raw else None

    def positions(self):
        q = urllib.parse.urlencode({"Recursive": "true", "IncludeItemTypes": "Movie", "SortBy": "SortName"})
        res = self.call("GET", f"/Users/{urllib.parse.quote(self.uid)}/Items?{q}")
        return {it["Id"]: {"name": it.get("Name", ""), "ticks": int((it.get("UserData") or {}).get("PlaybackPositionTicks") or 0)}
                for it in res["Items"]}

    def set_position(self, item_id, ticks):
        self.call("POST", "/Sessions/Playing/Stopped", {
            "ItemId": item_id, "MediaSourceId": item_id, "PlaySessionId": "resume-guard-" + uuid.uuid4().hex[:8],
            "PositionTicks": ticks, "IsPaused": False, "PlayMethod": "DirectPlay", "CanSeek": True})

    def logout(self):
        self.call("POST", "/Sessions/Logout")


def hms(ticks):
    s = ticks // TICKS
    return f"{s // 3600}:{s // 60 % 60:02d}:{s % 60:02d}"


def main():
    if len(sys.argv) != 3 or sys.argv[1] not in ("save", "restore"):
        sys.exit(__doc__)
    mode, path = sys.argv[1:]
    with open(os.environ["MOVIES_PASSWORD_FILE"]) as f:
        password = f.read().strip()
    m = Movies(os.environ["MOVIES_URL"], os.environ["MOVIES_USER"], password)
    try:
        now = m.positions()
        if mode == "save":
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            with os.fdopen(fd, "w") as f:
                json.dump(now, f, indent=1)
            for v in now.values():
                if v["ticks"]:
                    print(f"saved  {hms(v['ticks'])}  {v['name']}")
            print(f"{len(now)} films, {sum(1 for v in now.values() if v['ticks'])} with a resume point")
            return 0
        with open(path) as f:
            saved = json.load(f)
        for item_id, v in saved.items():
            if item_id in now and now[item_id]["ticks"] != v["ticks"]:
                m.set_position(item_id, v["ticks"])
                print(f"restored {hms(now[item_id]['ticks'])} -> {hms(v['ticks'])}  {v['name']}")
        after = m.positions()
        wrong = [v["name"] for i, v in saved.items() if i in after and after[i]["ticks"] != v["ticks"]]
        if wrong:
            print("NOT restored:", ", ".join(wrong))
            return 1
        print(f"all {len(saved)} positions match the saved state")
        return 0
    finally:
        m.logout()


if __name__ == "__main__":
    sys.exit(main())
