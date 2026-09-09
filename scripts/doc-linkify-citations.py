#!/usr/bin/env python3
"""One-shot: turn inline `file.go:N[-M]` doc citations into GitHub-anchored links.

    [`go/crdt/crdt.go:870-919`](../../go/crdt/crdt.go#L870-L919)

Bare filenames resolve against the page's package dir (docs/crdt → go/crdt …);
full paths are repo-root-relative. .go citations only — non-Go cites stay as-is.
The text inside the citation is not altered.

Idempotent: a citation already inside `` [`…`](…) `` link syntax is skipped.
"""
import re
import sys
from pathlib import Path

# Negative lookbehind on `[` immediately before the citation's opening backtick,
# so an already-linkified citation (its backtick span is link label text) is
# never wrapped again. Re-running on processed docs is a byte-identical no-op.
CITATION_RE = re.compile(
    r"(?<!\[)`((?:[a-zA-Z0-9_/.-]+/)?[a-zA-Z0-9_-]+\.go):(\d+)(?:-(\d+))?`")
PKG_DIRS = {"pheap": "pheap", "bxtree": "bxtree", "crdt": "crdt"}


def resolve(path: str, md_dir: Path, root: Path) -> str | None:
    root_rel = path if "/" in path else f"go/{PKG_DIRS[md_dir.name]}/{path}"
    if not (root / root_rel).exists():
        return None
    return root_rel


def depth(md: Path, root: Path) -> int:
    return max(1, len(md.parent.resolve().relative_to(root).parts))


def main() -> int:
    root = Path(__file__).resolve().parent.parent
    linkified = 0
    for md in sorted((root / "docs").rglob("*.md")):
        if "superpowers" in md.parts:
            continue
        lines = md.read_text(encoding="utf-8").split("\n")
        in_fence = False
        rel = "../" * depth(md, root)
        for i, line in enumerate(lines):
            if line.lstrip().startswith("```"):
                in_fence = not in_fence
                continue
            if in_fence:
                continue
            def repl(m, rel=rel):
                nonlocal linkified
                path, a, b = m.group(1), m.group(2), m.group(3) or m.group(2)
                frag = f"#L{a}" if b == a else f"#L{a}-L{b}"
                target = resolve(path, md.parent, root)
                if target is None:
                    return m.group(0)
                new = f"[`{m.group(0)[1:-1]}`]({rel}{target}{frag})"
                linkified += 1
                return new
            lines[i] = CITATION_RE.sub(repl, line)
        md.write_text("\n".join(lines), encoding="utf-8")
    print(f"linkified {linkified} citations")
    return 0


if __name__ == "__main__":
    sys.exit(main())
