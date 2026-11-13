#!/usr/bin/env python3
"""failed-tests.py <dir>...: the failed tests in Gradle's JUnit result files under <dir>, one line each
("Class.test: first line of the message"), and how many tests the files hold.

check-android.sh runs Gradle with -q, which then only says "56 tests completed, 2 failed"
(GitHub, 2026-11-13); this names them. Exit 1 when a test failed, 0 when none did, 2 when no
result file was found (a test task that did not run says nothing either)."""
import glob
import os
import sys
import xml.etree.ElementTree as ET

files = sorted(f for d in sys.argv[1:] for f in glob.glob(os.path.join(d, "**", "TEST-*.xml"), recursive=True))
if not files:
    print(f"failed-tests: no result file under {' '.join(sys.argv[1:])}")
    sys.exit(2)
total = 0
failed = []
for f in files:
    for case in ET.parse(f).getroot().iter("testcase"):
        total += 1
        for kind in ("failure", "error"):
            e = case.find(kind)
            if e is not None:
                text = (e.get("message") or e.text or "").strip().splitlines()
                failed.append(f"{case.get('classname', '').rsplit('.', 1)[-1]}.{case.get('name')}: {text[0][:300] if text else kind}")
                break
for line in failed:
    print("FAILED " + line)
print(f"failed-tests: {len(failed)} of {total} tests failed ({len(files)} result files)")
sys.exit(1 if failed else 0)
