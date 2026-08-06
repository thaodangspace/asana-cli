#!/usr/bin/env python3
"""Fail closed when a workflow introduces a mutable third-party action ref."""

from __future__ import annotations

import re
import sys
from pathlib import Path

USES_RE = re.compile(r"^\s*uses:\s*([^\s#]+)(?:\s+#\s*(v\S+))?\s*$")
SHA_RE = re.compile(r"^[0-9a-f]{40}$")


def main() -> None:
    root = Path(sys.argv[1] if len(sys.argv) > 1 else ".github/workflows")
    failures: list[str] = []
    for path in sorted(root.glob("*.y*ml")):
        for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            match = USES_RE.match(line)
            if not match:
                continue
            reference, comment = match.groups()
            if reference.startswith(("./", "docker://")):
                continue
            if "/" not in reference or "@" not in reference:
                failures.append(f"{path}:{line_number}: malformed action reference")
                continue
            _, ref = reference.rsplit("@", 1)
            if not SHA_RE.fullmatch(ref):
                failures.append(f"{path}:{line_number}: action is not pinned to a full SHA: {reference}")
            if not comment or not comment.startswith("v"):
                failures.append(f"{path}:{line_number}: SHA pin needs a readable version comment")
    if failures:
        print("GitHub Actions pin verification failed:", file=sys.stderr)
        print("\n".join(failures), file=sys.stderr)
        raise SystemExit(1)
    print(f"verified immutable action pins in {root}")


if __name__ == "__main__":
    main()
