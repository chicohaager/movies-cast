"""screen.py <what> [arg]: reads a uiautomator dump (stdin) for measure-user-ca.sh.

  texts            every text on the screen, one per line
  field <n>        the middle of the n-th text field from the top (0 = top), as "x y"
  tap <text>       the middle of the first text view whose text is exactly <text>, as "x y"
"""
import html
import re
import sys

dump = sys.stdin.read()
what = sys.argv[1]
NODE = re.compile(r'<node[^>]*?text="([^"]*)"[^>]*?class="([^"]*)"[^>]*?bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"')


def middle(m):
    return f"{(int(m.group(3)) + int(m.group(5))) // 2} {(int(m.group(4)) + int(m.group(6))) // 2}"


nodes = list(NODE.finditer(dump))
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
elif what == "tap":
    hits = [m for m in nodes if html.unescape(m.group(1)) == sys.argv[2] and m.group(2).endswith("TextView")]
    if not hits:
        sys.exit(f"no text '{sys.argv[2]}' on the screen")
    print(middle(hits[0]))
else:
    sys.exit("unknown: " + what)
