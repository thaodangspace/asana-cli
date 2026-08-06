#!/usr/bin/env python3
"""Create and validate the immutable manifest shipped with a release."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path

TARGETS = (("darwin", "amd64"), ("darwin", "arm64"), ("linux", "amd64"), ("linux", "arm64"))
ARCHIVE_RE = re.compile(r"^asana-cli_(?P<version>.+)_(?P<os>darwin|linux)_(?P<arch>amd64|arm64)\.tar\.gz$")


def fail(message: str) -> "NoReturn":
    print(f"release manifest failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def digest(path: Path) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            hasher.update(block)
    return hasher.hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("dist", type=Path)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--commit", required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:-(?:rc|beta)\.(?:0|[1-9]\d*))?", args.tag):
        fail("tag is not a validated release tag")
    version = args.tag[1:]
    if not args.dist.is_dir():
        fail(f"distribution directory is missing: {args.dist}")

    try:
        artifacts = json.loads((args.dist / "artifacts.json").read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read artifacts.json: {exc}")
    archives: dict[tuple[str, str], str] = {}
    for artifact in artifacts:
        if not isinstance(artifact, dict) or artifact.get("type") != "Archive":
            continue
        name = artifact.get("name")
        if not isinstance(name, str):
            fail("archive metadata has no name")
        match = ARCHIVE_RE.fullmatch(name)
        if not match or match.group("version") != version:
            fail(f"archive has an unexpected name: {name}")
        target = (match.group("os"), match.group("arch"))
        if target in archives:
            fail(f"duplicate archive for {target[0]}/{target[1]}")
        if not (args.dist / name).is_file():
            fail(f"archive is missing: {name}")
        archives[target] = name
    if set(archives) != set(TARGETS):
        fail("release must contain exactly the four supported archives")

    checksum_path = args.dist / "checksums.txt"
    if not checksum_path.is_file():
        fail("checksums.txt is missing")
    checksums: dict[str, str] = {}
    for line in checksum_path.read_text(encoding="utf-8").splitlines():
        fields = line.split()
        if len(fields) == 2:
            checksums[fields[1]] = fields[0]
    entries = []
    for target in TARGETS:
        name = archives[target]
        actual = digest(args.dist / name)
        if checksums.get(name) != actual:
            fail(f"checksum mismatch for {name}")
        entries.append({"name": name, "size": (args.dist / name).stat().st_size, "sha256": actual, "goos": target[0], "goarch": target[1]})

    manifest = {
        "schema_version": 1,
        "project": "asana-cli",
        "tag": args.tag,
        "version": version,
        "prerelease": "-" in version,
        "commit": args.commit,
        "artifacts": entries,
        "checksums": {"name": "checksums.txt", "sha256": digest(checksum_path), "size": checksum_path.stat().st_size},
    }
    (args.dist / "release-manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("release manifest verified")


if __name__ == "__main__":
    main()
