#!/usr/bin/env python3
"""android-screen.py <what> [arg]: what is on the screen of the device adb talks to (ANDROID_SERIAL).

  texts            every text on the screen, one per line
  field <n>        the middle of the n-th text field from the top (0 = top), as "x y"
  tap <text>       the middle of the first text view whose text is exactly <text>, as "x y"

Reads the screen with `uiautomator dump`. While a window opens that answers "null root node"
(Android 16 emulator, 2026-11-13), so it is asked again, up to 10 times, half a second apart.
"""
import html
import re
import subprocess
import sys
import time

NODE = re.compile(r'<node[^>]*?text="([^"]*)"[^>]*?class="([^"]*)"[^>]*?bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"')
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
    return f"{(int(m.group(3)) + int(m.group(5))) // 2} {(int(m.group(4)) + int(m.group(6))) // 2}"


def main() -> None:
    if len(sys.argv) < 2 or sys.argv[1] not in ("texts", "field", "tap"):
        sys.exit(__doc__)
    what = sys.argv[1]
    nodes = list(NODE.finditer(dump()))
    if what == "texts":
        for m in nodes:
            if m.group(1):
                print(html.unescape(m.group(1)))
    elif what == "field":
        fields = [m for m in nodes if m.group(2) == "android.widget.EditText"]
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
