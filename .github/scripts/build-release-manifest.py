#!/usr/bin/env python3
"""Create the machine-readable manifest from the final release bytes."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import sys
from pathlib import Path

TARGETS = (("darwin", "amd64"), ("darwin", "arm64"), ("linux", "amd64"), ("linux", "arm64"))
ARCHIVE_RE = re.compile(r"^asana-cli_(?P<version>.+)_(?P<os>darwin|linux)_(?P<arch>amd64|arm64)\.tar\.gz$")
SBOM_RE = re.compile(r"^asana-cli_(?P<version>.+)_(?P<os>darwin|linux)_(?P<arch>amd64|arm64)\.sbom\.spdx\.json$")
TAG_RE = re.compile(r"v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:-(?:rc|beta)\.(?:0|[1-9]\d*))?")


def fail(message: str) -> "NoReturn":
    print(f"release manifest failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def digest(path: Path) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            hasher.update(block)
    return hasher.hexdigest()


def file_entry(path: Path, kind: str, **extra: object) -> dict[str, object]:
    return {
        "filename": path.name,
        "name": path.name,
        "kind": kind,
        "sha256": digest(path),
        "size": path.stat().st_size,
        **extra,
    }


def checksum_map(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        fields = line.split()
        if len(fields) == 2 and re.fullmatch(r"[0-9a-f]{64}", fields[0]):
            values[fields[1]] = fields[0]
    return values


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("dist", type=Path)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--source-repository", default=os.environ.get("GITHUB_REPOSITORY", "thaodangspace/asana-cli"))
    parser.add_argument("--go-version", default=os.environ.get("GO_VERSION", "unknown"))
    parser.add_argument("--goreleaser-version", default=os.environ.get("GORELEASER_VERSION", "unknown"))
    parser.add_argument("--created-at", default=None)
    args = parser.parse_args()
    if not TAG_RE.fullmatch(args.tag):
        fail("tag is not a validated release tag")
    if not re.fullmatch(r"[0-9a-f]{40}", args.commit):
        fail("commit must be a full 40-character git SHA")
    if not args.dist.is_dir():
        fail(f"distribution directory is missing: {args.dist}")

    try:
        goreleaser_artifacts = json.loads((args.dist / "artifacts.json").read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read artifacts.json: {exc}")
    archives: dict[tuple[str, str], str] = {}
    for artifact in goreleaser_artifacts:
        if not isinstance(artifact, dict) or artifact.get("type") != "Archive":
            continue
        name = artifact.get("name")
        if not isinstance(name, str):
            fail("archive metadata has no name")
        match = ARCHIVE_RE.fullmatch(name)
        if not match or match.group("version") != args.tag[1:]:
            fail(f"archive has an unexpected name: {name}")
        target = (match.group("os"), match.group("arch"))
        if target in archives or not (args.dist / name).is_file():
            fail(f"archive is duplicated or missing: {name}")
        archives[target] = name
    if set(archives) != set(TARGETS):
        fail("release must contain exactly the four supported archives")

    checksum_path = args.dist / "checksums.txt"
    if not checksum_path.is_file():
        fail("checksums.txt is missing")
    checksums = checksum_map(checksum_path)
    entries: list[dict[str, object]] = []
    for target in TARGETS:
        name = archives[target]
        path = args.dist / name
        actual = digest(path)
        if checksums.get(name) != actual:
            fail(f"checksum mismatch for {name}")
        entries.append(
            file_entry(path, "archive", os=target[0], arch=target[1], goos=target[0], goarch=target[1])
        )

    sboms: dict[tuple[str, str], str] = {}
    for path in args.dist.glob("*.sbom.spdx.json"):
        match = SBOM_RE.fullmatch(path.name)
        if not match or match.group("version") != args.tag[1:]:
            fail(f"SBOM has an unexpected name: {path.name}")
        target = (match.group("os"), match.group("arch"))
        if target in sboms:
            fail(f"duplicate SBOM for {target[0]}/{target[1]}")
        try:
            document = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            fail(f"SBOM is not valid JSON: {path.name}: {exc}")
        if not isinstance(document, dict) or document.get("spdxVersion") not in {"SPDX-2.2", "SPDX-2.3"}:
            fail(f"SBOM is not SPDX JSON: {path.name}")
        if checksums.get(path.name) != digest(path):
            fail(f"checksum mismatch for {path.name}")
        sboms[target] = path.name
    if set(sboms) != set(TARGETS):
        fail("release must contain exactly one SPDX SBOM per supported target")
    for target in TARGETS:
        path = args.dist / sboms[target]
        entries.append(file_entry(path, "sbom", os=target[0], arch=target[1], format="SPDX-2.3 JSON"))

    created_at = args.created_at or dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z")
    try:
        dt.datetime.fromisoformat(created_at.replace("Z", "+00:00"))
    except ValueError:
        fail("created_at must be RFC3339")
    checksum_entry = file_entry(checksum_path, "checksum")
    entries.append(checksum_entry)
    manifest = {
        "schema_version": 1,
        "project": "asana-cli",
        "tag": args.tag,
        "version": args.tag[1:],
        "prerelease": "-" in args.tag,
        "commit": args.commit,
        "source_repository": args.source_repository,
        "go_version": args.go_version,
        "goreleaser_version": args.goreleaser_version,
        "created_at": created_at,
        "artifacts": entries,
        "checksums": checksum_entry,
    }
    (args.dist / "release-manifest.json").write_text(
        json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    print("release manifest verified")


if __name__ == "__main__":
    main()
