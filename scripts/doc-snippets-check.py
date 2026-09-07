#!/usr/bin/env python3
"""Verify embedded code snippets in docs match the source files they cite.

Fence format:
    ```go include go/crdt/crdt.go L440-L444 L448-L459
    <verbatim concatenation of the 1-based inclusive ranges,
     with a single `// …` line only BETWEEN ranges>
    ```
Path on the anchor line is repo-root-relative. Multiple ranges allowed;
the checker inserts no content for gap handling beyond the `// …` rule.
Collect all drifts, report them, exit 1 if any.
"""
import re
import sys
from pathlib import Path

def main() -> int:
    root = Path(__file__).resolve().parent.parent
    md_dirs = [root / "docs"]
    failures = []
    checked = 0
    for md in sorted(md_dirs[0].rglob("*.md")):
        if ".superpowers" in md.parts or "superpowers" in md.parts:
            continue
        text = md.read_text(encoding="utf-8")
        i = 0
        lines = text.splitlines()
        while i < len(lines):
            info = lines[i].strip()
            rest = info[6:].strip() if info.startswith("```go ") else None
            if not rest or not rest.startswith("include "):
                i += 1
                continue
            m = re.match(r"include (\S+)((?:\s+L\d+-L\d+)+)$", rest)
            if not m:
                i += 1
                continue
            path = m.group(1)
            ranges = [(int(a), int(b)) for a, b in
                      re.findall(r"L(\d+)-L(\d+)", m.group(2))]
            src = root / path
            if not src.exists():
                failures.append(f"{md}: source {path} does not exist")
                while i < len(lines) and not lines[i].strip().startswith("```"):
                    i += 1
                i += 1
                continue
            src_lines = src.read_text(encoding="utf-8").splitlines()
            expect = []
            for r_idx, (a, b) in enumerate(ranges):
                if r_idx:
                    expect.append("// …")
                if a < 1 or b > len(src_lines) or a > b:
                    failures.append(
                        f"{md}: fence at line {i+1} range L{a}-L{b} out of "
                        f"bounds for {path} ({len(src_lines)} lines)")
                    continue
                expect.extend(src_lines[a - 1 : b])
            body = []
            j = i + 1
            while j < len(lines) and not lines[j].strip() == "```":
                body.append(lines[j])
                j += 1
            got = "\n".join(expect)
            want = "\n".join(body)
            checked += 1
            if got != want:
                failures.append(
                    f"{md}: fence `` `go include {path}` `` drifts from source "
                    f"(expected {len(expect)} lines, found {len(body)}).\n"
                    f"--- expected ---\n{got}\n--- actual ---\n{want}")
            i = j + 1
    if failures:
        for f in failures:
            print("DRIFT:", f)
        print(f"{checked} snippets checked, {len(failures)} drifting")
        return 1
    print(f"{checked} snippets verified, all in sync")
    return 0

if __name__ == "__main__":
    sys.exit(main())
