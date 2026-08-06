#!/usr/bin/env python3
"""Verify the release snapshot artifact contract on the CI runner."""

from __future__ import annotations

import json
import re
import stat
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path


EXPECTED_TARGETS = {
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("linux", "amd64"),
    ("linux", "arm64"),
}
ARCHIVE_RE = re.compile(r"^asana-cli_(?P<version>.+)_(?P<goos>darwin|linux)_(?P<goarch>amd64|arm64)\.tar\.gz$")
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")


def fail(message: str) -> None:
    print(f"release snapshot verification failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def read_artifacts(dist: Path) -> list[dict[str, object]]:
    try:
        data = json.loads((dist / "artifacts.json").read_text())
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read dist/artifacts.json: {exc}")
    if not isinstance(data, list):
        fail("dist/artifacts.json is not an array")
    return [item for item in data if isinstance(item, dict)]


def verify_archives(dist: Path, artifacts: list[dict[str, object]]) -> tuple[dict[tuple[str, str], str], str]:
    found: dict[tuple[str, str], str] = {}
    versions: set[str] = set()
    for artifact in artifacts:
        if artifact.get("type") != "Archive":
            continue
        name = artifact.get("name")
        goos = artifact.get("goos")
        goarch = artifact.get("goarch")
        if not all(isinstance(value, str) for value in (name, goos, goarch)):
            fail("archive metadata is missing name, goos, or goarch")
        match = ARCHIVE_RE.fullmatch(name)
        if not match:
            fail(f"archive has an invalid name: {name}")
        target = (goos, goarch)
        if target not in EXPECTED_TARGETS:
            fail(f"unexpected archive target: {goos}/{goarch}")
        if target in found:
            fail(f"duplicate archive for {goos}/{goarch}")
        if not (dist / name).is_file():
            fail(f"archive listed in metadata is missing: {name}")
        found[target] = name
        versions.add(match.group("version"))

    if set(found) != EXPECTED_TARGETS:
        missing = ", ".join(f"{goos}/{goarch}" for goos, goarch in sorted(EXPECTED_TARGETS - set(found)))
        fail(f"expected exactly one archive for every target; missing: {missing}")
    if len(versions) != 1:
        fail("archives do not share one release version")
    return found, versions.pop()


def verify_checksums(dist: Path, archives: dict[tuple[str, str], str]) -> None:
    checksum_path = dist / "checksums.txt"
    if not checksum_path.is_file():
        fail("dist/checksums.txt is missing")
    checksums: dict[str, str] = {}
    for line in checksum_path.read_text().splitlines():
        fields = line.split()
        if len(fields) == 2 and SHA256_RE.fullmatch(fields[0]):
            checksums[fields[1]] = fields[0]
    for name in archives.values():
        if name not in checksums:
            fail(f"checksums.txt has no entry for {name}")


def verify_formula(dist: Path, archives: dict[tuple[str, str], str]) -> None:
    formula_path = dist / "homebrew" / "Formula" / "asana-cli.rb"
    if not formula_path.is_file():
        fail("generated Homebrew formula is missing")
    formula = formula_path.read_text()
    required = (
        "class AsanaCli < Formula",
        'desc "Asana CLI for agents and humans (JSON output by default)"',
        'homepage "https://github.com/thaodangspace/asana-cli"',
        'license "MIT"',
        'bin.install "asana-cli"',
        'system "#{bin}/asana-cli", "--version"',
    )
    for text in required:
        if text not in formula:
            fail(f"Homebrew formula is missing: {text}")

    urls = re.findall(r'^\s*url "([^"]+)"$', formula, re.MULTILINE)
    hashes = re.findall(r'^\s*sha256 "([0-9a-f]+)"$', formula, re.MULTILINE)
    if len(urls) != len(archives) or len(hashes) != len(archives):
        fail("Homebrew formula does not contain one URL and SHA-256 for each target")
    for name in archives.values():
        matching = [url for url in urls if url.endswith("/" + name)]
        if len(matching) != 1 or not matching[0].startswith("https://github.com/"):
            fail(f"Homebrew formula has no populated release URL for {name}")
    if any(not SHA256_RE.fullmatch(value) for value in hashes):
        fail("Homebrew formula contains an invalid SHA-256")


def verify_linux_binary(dist: Path, archives: dict[tuple[str, str], str]) -> None:
    archive_path = dist / archives[("linux", "amd64")]
    with tempfile.TemporaryDirectory() as temporary:
        destination = Path(temporary)
        with tarfile.open(archive_path, "r:gz") as archive:
            members = [member for member in archive.getmembers() if member.name == "asana-cli"]
            if len(members) != 1:
                fail("linux/amd64 archive does not contain exactly one asana-cli binary")
            member = members[0]
            if not member.isfile():
                fail("linux/amd64 asana-cli entry is not a regular file")
            archive.extract(member, destination)
        binary = destination / "asana-cli"
        binary.chmod(binary.stat().st_mode | stat.S_IXUSR)
        version = subprocess.run([str(binary), "--version"], check=False, capture_output=True, text=True)
        output = (version.stdout + version.stderr).strip()
        if version.returncode != 0 or not output or re.search(r"\bdev\b", output, re.IGNORECASE):
            fail("linux/amd64 snapshot does not report an injected non-dev version")
        help_result = subprocess.run([str(binary), "--help"], check=False, capture_output=True)
        if help_result.returncode != 0:
            fail("linux/amd64 snapshot --help failed")


def main() -> None:
    dist = Path(sys.argv[1] if len(sys.argv) > 1 else "dist")
    if not dist.is_dir():
        fail(f"distribution directory is missing: {dist}")
    artifacts = read_artifacts(dist)
    archives, _version = verify_archives(dist, artifacts)
    verify_checksums(dist, archives)
    verify_formula(dist, archives)
    verify_linux_binary(dist, archives)
    print("release snapshot artifact contract verified")


if __name__ == "__main__":
    main()
