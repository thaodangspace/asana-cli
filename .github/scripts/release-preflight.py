#!/usr/bin/env python3
"""Validate a release tag and emit the metadata used by later jobs."""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path

TAG_RE = re.compile(r"^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(rc|beta)\.(0|[1-9][0-9]*))?$")


def fail(message: str) -> "NoReturn":
    print(f"release preflight failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def git(*args: str) -> str:
    result = subprocess.run(["git", *args], check=False, capture_output=True, text=True)
    if result.returncode:
        fail(f"git {' '.join(args)} failed: {result.stderr.strip()}")
    return result.stdout.strip()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("tag")
    parser.add_argument("--output", default=os.environ.get("GITHUB_OUTPUT"))
    args = parser.parse_args()

    match = TAG_RE.fullmatch(args.tag)
    if not match:
        fail("tag must match vMAJOR.MINOR.PATCH, optionally followed by -rc.N or -beta.N")

    tag_commit = git("rev-parse", f"{args.tag}^{{commit}}")
    head = git("rev-parse", "HEAD")
    if tag_commit != head:
        fail(f"checked out commit {head} is not the tag target {tag_commit}")
    if git("show-ref", "--verify", f"refs/tags/{args.tag}") == "":
        fail("tag is not present in the complete checkout")
    if subprocess.run(["git", "cat-file", "-e", f"{args.tag}^{{commit}}"], check=False).returncode:
        fail("tag does not resolve to a commit")

    main_ref = "refs/remotes/origin/main"
    if subprocess.run(["git", "show-ref", "--verify", "--quiet", main_ref], check=False).returncode:
        fail("origin/main is unavailable; checkout must include complete history")
    if subprocess.run(["git", "merge-base", "--is-ancestor", tag_commit, main_ref], check=False).returncode:
        fail("tag target is not reachable from origin/main")

    version = args.tag[1:]
    prerelease = "true" if match.group(4) else "false"
    values = {
        "tag": args.tag,
        "version": version,
        "prerelease": prerelease,
        "commit": tag_commit,
    }
    if args.output:
        output = Path(args.output)
        with output.open("a", encoding="utf-8") as handle:
            for key, value in values.items():
                handle.write(f"{key}={value}\n")
    for key, value in values.items():
        print(f"{key}={value}")


if __name__ == "__main__":
    main()
