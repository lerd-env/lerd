#!/usr/bin/env python3
"""Checkbox coverage of tests/release-test-plan.md by the phase scripts.

usage: checkboxes.py list [phase...]        every checkbox with its id
       checkboxes.py coverage <log>...     which checkboxes the logs cover

Checkbox ids are <phase>.<n>, numbered in plan order. A check covers a
checkbox when its name starts with the id ("2.6 https answers 200").
"""
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
PLAN = HERE.parent / "release-test-plan.md"


def extract(plan_text):
    items, phase, n, current = [], None, 0, None
    for line in plan_text.splitlines():
        m = re.match(r"^## Phase (\d+)\b", line)
        if m:
            phase, n = int(m.group(1)), 0
            current = None
            continue
        if line.startswith("## "):
            phase, current = None, None
            continue
        if phase is None:
            continue
        m = re.match(r"^\s*- \[ \] (.*)$", line)
        if m:
            n += 1
            current = [f"{phase}.{n}", str(phase), m.group(1).strip()]
            items.append(current)
            continue
        # A wrapped checkbox continues on indented lines until a blank line.
        if current and line.startswith("      ") and line.strip():
            current[2] += " " + line.strip()
        else:
            current = None
    return items


RESULT = re.compile(r"^(PASS|FAIL|SKIP) (\d+\.\d+)\b( \[partial\])?")


def coverage(logs):
    items = extract(PLAN.read_text())
    seen = {}
    partial = set()
    for log in logs:
        for line in Path(log).read_text(errors="replace").splitlines():
            m = RESULT.match(line)
            if m:
                if m.group(3):
                    partial.add(m.group(2))
                # A FAIL anywhere wins over a PASS for the same checkbox.
                if seen.get(m.group(2)) != "FAIL":
                    seen[m.group(2)] = m.group(1)
    phases = sorted({int(i[1]) for i in items})
    missing_total = 0
    for phase in phases:
        mine = [i for i in items if int(i[1]) == phase]
        got = {k: v for k, v in seen.items() if k.split(".")[0] == str(phase)}
        missing = [i for i in mine if i[0] not in got]
        missing_total += len(missing)
        counts = {s: sum(1 for v in got.values() if v == s) for s in ("PASS", "FAIL", "SKIP")}
        part = sum(1 for k in got if k in partial)
        print(f"phase {phase:>2}: {len(mine):>3} checkboxes  {counts['PASS']:>3} pass  {counts['FAIL']:>3} fail  {counts['SKIP']:>3} skip  {part:>3} partial  {len(missing):>3} not covered")
        for i in missing:
            print(f"    {i[0]:<6} {i[2][:110]}")
    return 1 if missing_total else 0


def main():
    if len(sys.argv) < 2 or sys.argv[1] not in ("list", "coverage"):
        print(__doc__, file=sys.stderr)
        sys.exit(2)
    if sys.argv[1] == "list":
        want = set(sys.argv[2:])
        for i in extract(PLAN.read_text()):
            if not want or i[1] in want:
                print(f"{i[0]}\t{i[2]}")
        return
    sys.exit(coverage(sys.argv[2:]))


if __name__ == "__main__":
    main()
