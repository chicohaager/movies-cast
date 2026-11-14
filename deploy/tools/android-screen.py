#!/usr/bin/env python3
"""android-screen.py <what> [arg]: what is on the screen of the device adb talks to (ANDROID_SERIAL).

  texts            every text on the screen, one per line
  field <n>        the middle of the n-th text field from the top (0 = top), as "x y"
  tap <text>       the middle of the first text view whose text is exactly <text>, as "x y"
  type <label> <file>
                   types the content of <file> (first line) into the text field labelled <label> and checks it arrived

`type` reads the value from a file, so a password never stands in a command line of the caller. On a TV the app's
text fields stay read-only until OK is pressed on them (rememberRemoteField in ui/Common.kt), so after the tap no
keyboard came up and `input text` went nowhere (TV emulator API 30, 2026-11-14); `type` then presses OK
(DPAD_CENTER) and waits until the keyboard is shown. The check reads the field back: the same text, or for a
password field as many characters. What the field held before is deleted first.

Reads the screen with `uiautomator dump`. While a window opens that answers "null root node"
(Android 16 emulator, 2026-11-13), so it is asked again, up to 10 times, half a second apart.
"""
import html
import re
import shlex
import xml.etree.ElementTree as ET
import subprocess
import sys
import time

NODE = re.compile(r'<node[^>]*?text="([^"]*)"[^>]*?class="([^"]*)"[^>]*?password="(true|false)"[^>]*?bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"')
PLACE = "/sdcard/mc-ui.xml"


def adb(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(["adb", *args], capture_output=True, text=True, check=False)


def dump() -> str:
    for _ in range(10):
        adb("shell", "rm", "-f", PLACE)
        answer = adb("shell", "uiautomator", "dump", PLACE)
        if "dumped to" in answer.stdout + answer.stderr:
            return adb("shell", "cat", PLACE).stdout
        time.sleep(0.5)
    sys.exit("uiautomator gave no screen after 10 tries")


def middle(m: re.Match) -> str:
    return f"{(int(m.group(4)) + int(m.group(6))) // 2} {(int(m.group(5)) + int(m.group(7))) // 2}"


def for_input_text(value: str) -> str:
    """The value as one argument for `adb shell input text`: the device's shell reads it once more, so it is quoted
    for that shell, and a space is written %s, which `input text` turns back into a space."""
    if "%s" in value:
        sys.exit("a value with '%s' in it cannot be typed by `input text`")
    return shlex.quote(value.replace(" ", "%s"))


def text_fields(nodes: list) -> list:
    return [m for m in nodes if m.group(2) == "android.widget.EditText"]


def labelled_field(screen: str, label: str):
    """The text field whose label (a text view inside it, as Compose's OutlinedTextField writes it) is <label>, or
    None. A field is found by its label, not by its place: while the keyboard is open the page scrolls and a field
    above leaves the dump (login page, Android TV emulator, 2026-11-14: 3 fields before, 2 after)."""
    for node in ET.fromstring(screen).iter("node"):
        if node.get("class") != "android.widget.EditText":
            continue
        if any(child.get("text") == label for child in node.iter("node") if child is not node):
            return node
    return None


def keyboard_shown() -> bool:
    return "mInputShown=true" in adb("shell", "dumpsys", "input_method").stdout


def type_into(label: str, value: str) -> None:
    field = labelled_field(dump(), label)
    if field is None:
        sys.exit(f"no text field labelled '{label}' on the screen")
    x1, y1, x2, y2 = map(int, re.findall(r"\d+", field.get("bounds")))
    adb("shell", "input", "tap", str((x1 + x2) // 2), str((y1 + y2) // 2))
    # Typed before the field is tied to the keyboard, the text goes nowhere (phone emulator, 2026-11-14: right after
    # the app's start, 0.5 s after the tap). So wait until the field has the focus and the keyboard is shown; on a TV
    # the keyboard comes only with OK.
    def ready() -> bool:
        node = labelled_field(dump(), label)
        return node is not None and node.get("focused") == "true" and keyboard_shown()
    # OK only on a TV: on a phone OK in a text field can act as Enter (send a search, a login)
    tv = adb("shell", "pm", "has-feature", "android.software.leanback").stdout.strip() == "true"
    pressed_ok = False
    for attempt in range(24):
        if ready():
            break
        if tv and attempt == 4 and not pressed_ok:
            adb("shell", "input", "keyevent", "DPAD_CENTER")
            pressed_ok = True
        time.sleep(0.25)
    else:
        sys.exit(f"text field '{label}': no keyboard came up after the tap and OK")
    time.sleep(0.3)
    # The value replaces what the field holds (a login page keeps the last address and name): to its end, then one
    # delete per character the dump shows (a password field shows one dot per character).
    old = len(field.get("text", ""))
    if old:
        adb("shell", "input", "keyevent", "MOVE_END", *(["DEL"] * old))
        time.sleep(0.3)
    # Even with the field focused and the keyboard shown, text sent at once can be lost while the keyboard is still
    # being tied to the field (phone emulator, 2026-11-14). So the arrival is what counts: read the field back for up to
    # 2 s; only when nothing at all arrived is it typed once more (never after part of it arrived: no doubled text).
    def arrived():
        node = labelled_field(dump(), label)
        return node, (len(node.get("text", "")) if node is not None else -1)
    field = None
    for typing in range(2):
        adb("shell", "input", "text", for_input_text(value))
        for _ in range(8):
            time.sleep(0.25)
            field, count = arrived()
            if count != 0:
                break
        if field is None or len(field.get("text", "")) != 0:
            break
    if field is None:
        sys.exit(f"text field '{label}' is gone from the screen after typing")
    got = field.get("text", "")
    if field.get("password") == "true":
        if len(got) != len(value):
            sys.exit(f"text field '{label}' (password): {len(got)} characters arrived, {len(value)} typed")
    elif got != value:
        # no value in the message: a password field the dump does not mark as one would print the secret
        sys.exit(f"text field '{label}': it shows {len(got)} characters that differ from the {len(value)} typed")
    # BACK closes the keyboard, so the next field and the buttons below are free; only while it is open, else BACK
    # would leave the page (ESCAPE left it open). An app that takes this BACK for itself shows here: the field changed.
    if keyboard_shown():
        adb("shell", "input", "keyevent", "BACK")
        time.sleep(0.5)
        after = labelled_field(dump(), label)
        if after is None or after.get("text", "") != got:
            sys.exit(f"text field '{label}': typed and read back, but BACK (to close the keyboard) changed it or the page")
    print(f"text field '{label}': {len(value)} characters typed and read back")


def main() -> None:
    if len(sys.argv) < 2 or sys.argv[1] not in ("texts", "field", "tap", "type"):
        sys.exit(__doc__)
    what = sys.argv[1]
    if what == "type":
        if len(sys.argv) != 4:
            sys.exit(__doc__)
        with open(sys.argv[3], encoding="utf-8") as f:
            value = f.readline().rstrip("\n")
        type_into(sys.argv[2], value)
        return
    nodes = list(NODE.finditer(dump()))
    if what == "texts":
        for m in nodes:
            if m.group(1):
                print(html.unescape(m.group(1)))
    elif what == "field":
        fields = text_fields(nodes)
        n = int(sys.argv[2])
        if n >= len(fields):
            sys.exit(f"no text field {n} on the screen ({len(fields)} there)")
        print(middle(fields[n]))
    else:
        hits = [m for m in nodes if html.unescape(m.group(1)) == sys.argv[2] and m.group(2).endswith("TextView")]
        if not hits:
            sys.exit(f"no text '{sys.argv[2]}' on the screen")
        print(middle(hits[0]))


main()
