#!/usr/bin/env python3
"""Verify final release archives, SBOMs, manifest, and checksums."""

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


def safe_path(dist: Path, name: object) -> Path:
    if not isinstance(name, str) or not name or Path(name).name != name:
        fail("manifest contains an unsafe filename")
    path = dist / name
    if not path.is_file():
        fail(f"artifact is missing: {name}")
    return path


def verify_spdx(path: Path) -> None:
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"SBOM is not valid JSON: {path.name}: {exc}")
    if not isinstance(document, dict) or document.get("spdxVersion") not in {"SPDX-2.2", "SPDX-2.3"}:
        fail(f"SBOM is not SPDX JSON: {path.name}")
    for key in ("SPDXID", "name", "documentNamespace", "creationInfo", "packages"):
        if key not in document:
            fail(f"SBOM is missing {key}: {path.name}")


def verify_linux_binary(dist: Path, name: str, version: str) -> None:
    with tempfile.TemporaryDirectory() as temporary:
        destination = Path(temporary)
        try:
            with tarfile.open(dist / name, "r:gz") as archive:
                members = archive.getmembers()
                if not members or set(member.name for member in members) - {"asana-cli", "LICENSE", "README.md"} or sum(member.name == "asana-cli" for member in members) != 1:
                    fail("linux/amd64 archive contains files outside the binary and permitted metadata")
                binary_member = next(member for member in members if member.name == "asana-cli")
                if not binary_member.isfile():
                    fail("linux/amd64 asana-cli is not a regular file")
                archive.extract(binary_member, destination)
        except (OSError, tarfile.TarError) as exc:
            fail(f"cannot inspect linux/amd64 archive: {exc}")
        binary = destination / "asana-cli"
        binary.chmod(binary.stat().st_mode | stat.S_IXUSR)
        result = subprocess.run([str(binary), "--version"], check=False, capture_output=True, text=True)
        output = (result.stdout + result.stderr).strip()
        if result.returncode or not re.search(rf"(?<![0-9A-Za-z]){re.escape(version)}(?![0-9A-Za-z])", output):
            fail(f"released Linux binary reports the wrong version: {output}")
        if subprocess.run([str(binary), "--help"], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode:
            fail("released Linux binary --help failed")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("dist", type=Path)
    parser.add_argument("--expected-version")
    parser.add_argument("--repository", default=None)
    args = parser.parse_args()
    try:
        manifest = json.loads((args.dist / "release-manifest.json").read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read release-manifest.json: {exc}")
    if not isinstance(manifest, dict) or manifest.get("schema_version") != 1:
        fail("manifest has an unsupported schema")
    if args.expected_version and manifest.get("version") != args.expected_version.removeprefix("v"):
        fail("manifest version does not match the release tag")
    if args.repository and manifest.get("source_repository") != args.repository:
        fail("manifest source repository is unexpected")
    if not re.fullmatch(r"[0-9a-f]{40}", str(manifest.get("commit", ""))):
        fail("manifest commit is not a full git SHA")
    artifacts = manifest.get("artifacts")
    if not isinstance(artifacts, list) or len(artifacts) != 9:
        fail("manifest must contain four archives, four SBOMs, and checksums.txt")

    checksums_path = args.dist / "checksums.txt"
    if not checksums_path.is_file():
        fail("checksums.txt is missing")
    checksums: dict[str, str] = {}
    for line in checksums_path.read_text(encoding="utf-8").splitlines():
        fields = line.split()
        if len(fields) == 2:
            checksums[fields[1]] = fields[0]

    archives: dict[tuple[str, str], str] = {}
    sboms: dict[tuple[str, str], str] = {}
    names: set[str] = set()
    for artifact in artifacts:
        if not isinstance(artifact, dict):
            fail("manifest contains an invalid artifact")
        name = artifact.get("filename", artifact.get("name"))
        path = safe_path(args.dist, name)
        if name in names:
            fail(f"manifest contains a duplicate filename: {name}")
        names.add(name)
        if not isinstance(artifact.get("size"), int) or path.stat().st_size != artifact["size"]:
            fail(f"artifact size is wrong: {name}")
        actual = digest(path)
        if actual != artifact.get("sha256"):
            fail(f"artifact digest is wrong: {name}")
        kind = artifact.get("kind")
        if kind == "archive":
            target = (artifact.get("os", artifact.get("goos")), artifact.get("arch", artifact.get("goarch")))
            if target not in TARGETS or target in archives:
                fail("manifest contains an invalid or duplicate archive target")
            archives[target] = name
        elif kind == "sbom":
            target = (artifact.get("os"), artifact.get("arch"))
            if target not in TARGETS or target in sboms:
                fail("manifest contains an invalid or duplicate SBOM target")
            verify_spdx(path)
            sboms[target] = name
        elif kind != "checksum" or name != "checksums.txt":
            fail(f"manifest contains an unknown artifact kind: {name}")

    if set(archives) != TARGETS or set(sboms) != TARGETS:
        fail("manifest must contain exactly one archive and SBOM for every target")
    checksum_entry = manifest.get("checksums")
    if not isinstance(checksum_entry, dict) or checksum_entry.get("filename", checksum_entry.get("name")) != "checksums.txt":
        fail("manifest checksums metadata is missing")
    if digest(checksums_path) != checksum_entry.get("sha256") or checksums_path.stat().st_size != checksum_entry.get("size"):
        fail("checksums.txt digest or size does not match the manifest")
    for name in list(archives.values()) + list(sboms.values()):
        artifact = next(item for item in artifacts if item.get("filename", item.get("name")) == name)
        if checksums.get(name) != artifact["sha256"]:
            fail(f"checksums.txt disagrees with the manifest for {name}")

    verify_linux_binary(args.dist, archives[("linux", "amd64")], str(manifest["version"]))
    print("final release artifact, SBOM, and manifest contract verified")


if __name__ == "__main__":
    main()
