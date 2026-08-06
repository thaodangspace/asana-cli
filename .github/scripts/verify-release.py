#!/usr/bin/env python3
"""Verify final release artifacts before and after they are published."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import stat
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path

TARGETS = {("darwin", "amd64"), ("darwin", "arm64"), ("linux", "amd64"), ("linux", "arm64")}


def fail(message: str) -> "NoReturn":
    print(f"release verification failed: {message}", file=sys.stderr)
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
    parser.add_argument("--expected-version")
    args = parser.parse_args()
    try:
        manifest = json.loads((args.dist / "release-manifest.json").read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read release-manifest.json: {exc}")
    if args.expected_version and manifest.get("version") != args.expected_version.removeprefix("v"):
        fail("manifest version does not match the release tag")
    artifacts = manifest.get("artifacts")
    if not isinstance(artifacts, list) or len(artifacts) != 4:
        fail("manifest must contain exactly four archives")
    found: set[tuple[str, str]] = set()
    for artifact in artifacts:
        if not isinstance(artifact, dict):
            fail("manifest contains an invalid artifact")
        target = (artifact.get("goos"), artifact.get("goarch"))
        name = artifact.get("name")
        if target in found or target not in TARGETS or not isinstance(name, str):
            fail("manifest contains an invalid or duplicate target")
        found.add(target)
        path = args.dist / name
        if not path.is_file() or path.stat().st_size != artifact.get("size"):
            fail(f"artifact size is wrong or file is missing: {name}")
        if digest(path) != artifact.get("sha256"):
            fail(f"artifact digest is wrong: {name}")

    checksum_path = args.dist / "checksums.txt"
    if not checksum_path.is_file():
        fail("checksums.txt is missing")
    if digest(checksum_path) != manifest.get("checksums", {}).get("sha256"):
        fail("checksums.txt digest does not match the manifest")
    checksums = {}
    for line in checksum_path.read_text(encoding="utf-8").splitlines():
        fields = line.split()
        if len(fields) == 2:
            checksums[fields[1]] = fields[0]
    for artifact in artifacts:
        if checksums.get(artifact["name"]) != artifact["sha256"]:
            fail(f"checksums.txt disagrees with the manifest for {artifact['name']}")

    linux_name = next(item["name"] for item in artifacts if item["goos"] == "linux" and item["goarch"] == "amd64")
    expected = manifest["version"]
    with tempfile.TemporaryDirectory() as temporary:
        destination = Path(temporary)
        with tarfile.open(args.dist / linux_name, "r:gz") as archive:
            members = archive.getmembers()
            if not members or set(member.name for member in members) - {"asana-cli", "LICENSE", "README.md"} or sum(member.name == "asana-cli" for member in members) != 1:
                fail("linux/amd64 archive contains files outside the binary and permitted metadata")
            binary_member = next(member for member in members if member.name == "asana-cli")
            if not binary_member.isfile():
                fail("linux/amd64 asana-cli is not a regular file")
            archive.extract(binary_member, destination)
        binary = destination / "asana-cli"
        binary.chmod(binary.stat().st_mode | stat.S_IXUSR)
        result = subprocess.run([str(binary), "--version"], check=False, capture_output=True, text=True)
        output = (result.stdout + result.stderr).strip()
        if result.returncode or not re.search(rf"(?<![0-9A-Za-z]){re.escape(expected)}(?![0-9A-Za-z])", output):
            fail(f"released Linux binary reports the wrong version: {output}")
        if subprocess.run([str(binary), "--help"], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode:
            fail("released Linux binary --help failed")
    print("final release artifact contract verified")


if __name__ == "__main__":
    main()
