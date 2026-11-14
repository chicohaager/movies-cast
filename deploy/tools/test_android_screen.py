"""Tests for `android-screen.py type` against a fake adb (made-up values only).

    python3 deploy/tools/test_android_screen.py      exit 1 on any failure

The fake adb keeps a screen of labelled text fields in a JSON file and acts like the devices measured on
2026-11-14: "tv" opens the keyboard only on OK (DPAD_CENTER) after the tap, as the app's fields on the Android TV
emulator (API 30) did; "phone" opens it on the tap. While the keyboard is open the page scrolls and the first field
leaves the dump, as the login page did; ESCAPE leaves the keyboard open, BACK closes it, and BACK without a keyboard
leaves the page. `input text` reaches the focused field only while the keyboard is shown, and its argument is read
once more by a shell, as on the device.

Cases: a keyboard that comes up late is waited for; a first typing that is lost is typed once more, a slow one is not doubled; a filled field (also a password field) gets the new value in place of the old; TV needs OK and the value arrives; phone gets no OK; the last field is found by its label while the first
has scrolled away, and only that field gets the value; the keyboard is closed and the page not left; spaces and
shell signs arrive as typed; a password field is checked by its length and the value is in no output; no keyboard
is a failure; a value that does not arrive is a failure without the value in the message; an unknown label is a
failure.
"""
import json
import os
import pathlib
import subprocess
import sys
import tempfile

TOOL = pathlib.Path(__file__).resolve().parent / "android-screen.py"
SECRET = "made-up pass;word&'x"  # made up

FAKE_ADB = r'''#!/usr/bin/env python3
import json, os, shlex, sys
state_file = os.environ["FAKE_ADB_STATE"]
st = json.load(open(state_file))
args = sys.argv[1:]
with open(state_file + ".log", "a") as log:
    log.write(" ".join(args) + "\n")
def save():
    json.dump(st, open(state_file, "w"))
def esc(t):
    return t.replace("&", "&amp;").replace('"', "&quot;").replace("'", "&apos;").replace("<", "&lt;")
def xml():
    out = []
    st["dumps"] = st.get("dumps", 0) + 1
    save()
    for i, f in enumerate(st["fields"]):
        if st["keyboard"] and i == 0 and len(st["fields"]) > 1:
            continue  # scrolled out of the dump while the keyboard is open
        shown = f["text"]
        if st.get("shows_after") and st["dumps"] - st.get("typed_at", 0) < st["shows_after"]:
            shown = f.get("before", "")  # the dump still shows the field as it was before the typing
        text = "•" * len(shown) if f["password"] else shown
        y = 100 + i * 100
        out.append('<node index="%d" text="%s" resource-id="" class="android.widget.EditText" package="x" '
                   'content-desc="" focused="%s" password="%s" selected="false" bounds="[0,%d][200,%d]">'
                   '<node index="0" text="%s" resource-id="" class="android.widget.TextView" package="x" '
                   'content-desc="" focused="false" password="false" selected="false" bounds="[10,%d][100,%d]" /></node>'
                   % (i, esc(text), str(st["focus"] == i).lower(), str(f["password"]).lower(), y, y + 80,
                      esc(f["label"]), y, y + 20))
    return '<?xml version="1.0" encoding="UTF-8"?><hierarchy rotation="0">' + "".join(out) + "</hierarchy>"
if args[:2] == ["shell", "rm"]:
    pass
elif args[:3] == ["shell", "pm", "has-feature"]:
    print("true" if st["device"] == "tv" else "false")
elif args[:2] == ["shell", "uiautomator"]:
    print("UI hierchary dumped to: /sdcard/mc-ui.xml")
elif args[:2] == ["shell", "cat"]:
    print(xml())
elif args[:3] == ["shell", "dumpsys", "input_method"]:
    # a keyboard that comes up late: shown only from the n-th question on
    st["asked"] = st.get("asked", 0) + 1
    late = st["keyboard"] and st["asked"] < st.get("keyboard_after", 0)
    print("  mInputShown=%s" % str(st["keyboard"] and not late).lower())
    save()
elif args[:3] == ["shell", "input", "tap"]:
    y = int(args[4])
    st["focus"] = (y - 100) // 100
    if st["device"] == "phone" and not st.get("no_keyboard"):
        st["keyboard"] = True
    save()
elif args[:3] == ["shell", "input", "keyevent"]:
    if args[3] == "DPAD_CENTER" and not st.get("no_keyboard"):
        st["keyboard"] = True
    elif args[3] == "MOVE_END":
        for key in args[4:]:
            assert key == "DEL", args
            if st["keyboard"] and st["focus"] is not None:
                st["fields"][st["focus"]]["text"] = st["fields"][st["focus"]]["text"][:-1]
    elif args[3] == "BACK":
        if st.get("back_clears"):
            # an app that takes BACK for itself: the field is emptied, the keyboard stays
            st["fields"][st["focus"]]["text"] = ""
        elif st["keyboard"]:
            st["keyboard"] = False
        else:
            st["left_page"] = True
    save()
elif args[:3] == ["shell", "input", "text"]:
    words = shlex.split(args[3])  # the device's shell
    assert len(words) == 1, words
    late = st.get("asked", 0) < st.get("keyboard_after", 0)
    st["typed"] = st.get("typed", 0) + 1
    lost = st.get("lose_first") and st["typed"] == 1
    if st["focus"] is not None:
        st["fields"][st["focus"]]["before"] = st["fields"][st["focus"]]["text"]
        st["typed_at"] = st.get("dumps", 0)
    if st["keyboard"] and not late and not lost and st["focus"] is not None and not st.get("drop"):
        st["fields"][st["focus"]]["text"] += words[0].replace("%s", " ")
    save()
else:
    sys.exit("fake adb: unknown call %r" % args)
'''


def run(case: dict, value: str, label: str = "Server address"):
    with tempfile.TemporaryDirectory() as d:
        bin_dir = pathlib.Path(d, "bin")
        bin_dir.mkdir()
        adb = bin_dir / "adb"
        adb.write_text(FAKE_ADB)
        adb.chmod(0o755)
        state = pathlib.Path(d, "state.json")
        state.write_text(json.dumps({"focus": None, "keyboard": False, **case}))
        value_file = pathlib.Path(d, "value")
        value_file.write_text(value + "\n")
        env = {**os.environ, "PATH": f"{bin_dir}:{os.environ['PATH']}", "FAKE_ADB_STATE": str(state)}
        p = subprocess.run([sys.executable, str(TOOL), "type", label, str(value_file)],
                           capture_output=True, text=True, env=env, timeout=60, check=False)
        log_file = pathlib.Path(str(state) + ".log")
        log = log_file.read_text() if log_file.exists() else ""
        return p, json.loads(state.read_text()), log


def field(label="Server address", password=False, text=""):
    return {"text": text, "password": password, "label": label}


def login_page():
    return [field(), field("User name"), field("Password", password=True)]


failures = []


def check(name, cond, detail=""):
    print(("ok   " if cond else "FAIL ") + name + ("" if cond else f": {detail}"))
    if not cond:
        failures.append(name)


p, st, log = run({"device": "tv", "fields": [field(), field("User name")]}, "192.0.2.17", label="User name")
check("tv: value arrives in the labelled field", p.returncode == 0 and st["fields"][1]["text"] == "192.0.2.17", p.stderr)
check("tv: OK was pressed", "keyevent DPAD_CENTER" in log, log)
check("tv: keyboard closed afterwards, page not left", st["keyboard"] is False and not st.get("left_page"), str(st))

p, st, log = run({"device": "phone", "fields": [field()]}, "abc")
check("phone: value arrives", p.returncode == 0 and st["fields"][0]["text"] == "abc", p.stderr)
check("phone: no OK pressed", "DPAD_CENTER" not in log, log)

# the keyboard is tied to the field only a while after the tap: text sent before that is lost
p, st, log = run({"device": "phone", "keyboard_after": 6, "fields": [field()]}, "abc")
check("phone, keyboard late: waits and the value arrives", p.returncode == 0 and st["fields"][0]["text"] == "abc", p.stderr)
check("phone, keyboard late: no OK pressed (it could send the form)", "DPAD_CENTER" not in log, log)

# the first typing is lost although field and keyboard look ready: typed once more, the value arrives once
p, st, log = run({"device": "phone", "lose_first": True, "fields": [field()]}, "abc")
check("first typing lost: typed again, arrives once", p.returncode == 0 and st["fields"][0]["text"] == "abc", f"{p.stderr} got {st['fields'][0]['text']!r}")
check("first typing lost: two typings", log.count("input text") == 2, log)

# the text arrives but the dump shows it only a moment later: not typed twice
p, st, log = run({"device": "phone", "shows_after": 3, "fields": [field()]}, "abc")
check("slow to show: typed once, not doubled", p.returncode == 0 and st["fields"][0]["text"] == "abc" and log.count("input text") == 1,
      f"{p.stderr} got {st['fields'][0]['text']!r}, {log.count('input text')} typings")

p, st, _ = run({"device": "tv", "fields": login_page()}, SECRET, label="Password")
check("login page: password arrives while the first field scrolled away",
      p.returncode == 0 and st["fields"][2]["text"] == SECRET, p.stderr)
check("login page: only the password field got it",
      st["fields"][0]["text"] == "" and st["fields"][1]["text"] == "", str(st["fields"][:2]))
check("password: value in no output", SECRET not in p.stdout + p.stderr)

p, st, _ = run({"device": "tv", "fields": [field(text="192.0.2.71"), field("User name"),
                                           field("Password", password=True, text="old-pass")]}, SECRET, label="Password")
check("filled password field: the old value is replaced", p.returncode == 0 and st["fields"][2]["text"] == SECRET,
      f"{p.stderr} got {len(st['fields'][2]['text'])} characters")
p, st, _ = run({"device": "phone", "fields": [field(text="192.0.2.71")]}, "192.0.2.17")
check("filled field: the old value is replaced", p.returncode == 0 and st["fields"][0]["text"] == "192.0.2.17",
      f"{p.stderr} got {st['fields'][0]['text']!r}")

odd = "a b;c&d'e\"f$g"
p, st, _ = run({"device": "tv", "fields": [field()]}, odd)
check("spaces and shell signs arrive as typed", p.returncode == 0 and st["fields"][0]["text"] == odd,
      f"{p.stderr} got {st['fields'][0]['text']!r}")

p, _, _ = run({"device": "tv", "no_keyboard": True, "fields": [field()]}, "abc")
check("no keyboard: fails", p.returncode != 0 and "no keyboard" in p.stderr, p.stderr)

p, _, _ = run({"device": "tv", "drop": True, "fields": [field()]}, "visible-value")
check("value lost: fails", p.returncode != 0 and "differ" in p.stderr, p.stderr)
check("value lost: value not in the message", "visible-value" not in p.stdout + p.stderr, p.stderr)

p, _, _ = run({"device": "tv", "drop": True, "fields": login_page()}, SECRET, label="Password")
check("password lost: fails without the value", p.returncode != 0 and SECRET not in p.stdout + p.stderr, p.stderr)

p, _, _ = run({"device": "phone", "back_clears": True, "fields": [field("Search")]}, "Rocky", label="Search")
check("BACK that changes the field: fails and says so", p.returncode != 0 and "BACK" in p.stderr, p.stderr)

p, _, _ = run({"device": "tv", "fields": [field()]}, "abc", label="Nickname")
check("unknown label: fails", p.returncode != 0 and "no text field labelled 'Nickname'" in p.stderr, p.stderr)

# `field` and `tap` (used by deploy/tools/user-ca/measure-user-ca.sh) read the same dump: the middle of a field and of
# a label as "x y".
def call(case: dict, *args: str):
    with tempfile.TemporaryDirectory() as d:
        bin_dir = pathlib.Path(d, "bin")
        bin_dir.mkdir()
        (bin_dir / "adb").write_text(FAKE_ADB)
        (bin_dir / "adb").chmod(0o755)
        state = pathlib.Path(d, "state.json")
        state.write_text(json.dumps({"focus": None, "keyboard": False, **case}))
        env = {**os.environ, "PATH": f"{bin_dir}:{os.environ['PATH']}", "FAKE_ADB_STATE": str(state)}
        return subprocess.run([sys.executable, str(TOOL), *args], capture_output=True, text=True, env=env, timeout=60, check=False)


p = call({"device": "tv", "fields": login_page()}, "field", "1")
check("field 1: the middle of the second field", p.returncode == 0 and p.stdout.strip() == "100 240", p.stdout + p.stderr)
p = call({"device": "tv", "fields": login_page()}, "tap", "Password")
check("tap: the middle of a label", p.returncode == 0 and p.stdout.strip() == "55 310", p.stdout + p.stderr)

print(f"{len(failures)} failure(s)")
sys.exit(1 if failures else 0)
