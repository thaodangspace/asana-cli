#!/usr/bin/env python3
"""Generate one SPDX JSON SBOM for every release archive.

The workflow installs Syft at a separately pinned version. This script keeps
that tool invocation and the archive-to-SBOM naming contract in one place,
then extends checksums.txt so every published SBOM is covered.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
from pathlib import Path

ARCHIVE_RE = re.compile(
    r"^asana-cli_(?P<version>.+)_(?P<os>darwin|linux)_(?P<arch>amd64|arm64)\.tar\.gz$"
)
SBOM_RE = re.compile(
    r"^asana-cli_(?P<version>.+)_(?P<os>darwin|linux)_(?P<arch>amd64|arm64)\.sbom\.spdx\.json$"
)
TARGETS = {("darwin", "amd64"), ("darwin", "arm64"), ("linux", "amd64"), ("linux", "arm64")}


def fail(message: str) -> "NoReturn":
    print(f"release SBOM generation failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def read_archive_names(dist: Path) -> dict[tuple[str, str], str]:
    try:
        metadata = json.loads((dist / "artifacts.json").read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read artifacts.json: {exc}")
    if not isinstance(metadata, list):
        fail("artifacts.json is not an array")

    archives: dict[tuple[str, str], str] = {}
    for artifact in metadata:
        if not isinstance(artifact, dict) or artifact.get("type") != "Archive":
            continue
        name = artifact.get("name")
        if not isinstance(name, str):
            fail("archive metadata has no name")
        match = ARCHIVE_RE.fullmatch(name)
        if not match:
            fail(f"archive has an unexpected name: {name}")
        target = (match.group("os"), match.group("arch"))
        if target in archives:
            fail(f"duplicate archive for {target[0]}/{target[1]}")
        if not (dist / name).is_file():
            fail(f"archive is missing: {name}")
        archives[target] = name

    if set(archives) != TARGETS:
        fail("release must contain exactly the four supported archives")
    return archives


def validate_sbom(path: Path, archive: Path, forbidden: list[str]) -> None:
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"SBOM for {archive.name} is not valid JSON: {exc}")
    if not isinstance(document, dict) or document.get("spdxVersion") not in {"SPDX-2.2", "SPDX-2.3"}:
        fail(f"SBOM for {archive.name} is not SPDX JSON")
    for key in ("SPDXID", "name", "documentNamespace", "creationInfo", "packages"):
        if key not in document:
            fail(f"SBOM for {archive.name} is missing {key}")
    serialized = json.dumps(document, sort_keys=True)
    for value in forbidden:
        if value and value in serialized:
            fail(f"SBOM for {archive.name} contains a runner-local or secret value")


def update_checksums(dist: Path, names: list[str]) -> None:
    checksum_path = dist / "checksums.txt"
    try:
        lines = checksum_path.read_text(encoding="utf-8").splitlines()
    except OSError as exc:
        fail(f"cannot read checksums.txt: {exc}")
    checksums: dict[str, str] = {}
    for line in lines:
        fields = line.split()
        if len(fields) == 2 and re.fullmatch(r"[0-9a-f]{64}", fields[0]):
            checksums[fields[1]] = fields[0]
    for name in names:
        path = dist / name
        actual = sha256(path)
        if name in checksums and checksums[name] != actual:
            fail(f"existing checksum disagrees with {name}")
        checksums[name] = actual
    checksum_path.write_text(
        "".join(f"{checksums[name]}  {name}\n" for name in sorted(checksums)),
        encoding="utf-8",
    )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("dist", type=Path)
    parser.add_argument("--tool", default="syft")
    parser.add_argument("--tool-version", default=os.environ.get("SYFT_VERSION", ""))
    args = parser.parse_args()
    if not args.dist.is_dir():
        fail(f"distribution directory is missing: {args.dist}")

    archives = read_archive_names(args.dist)
    if args.tool_version:
        version = subprocess.run(
            [args.tool, "version"], check=False, capture_output=True, text=True
        )
        if version.returncode != 0 or args.tool_version not in version.stdout + version.stderr:
            fail(f"{args.tool} is not the pinned version {args.tool_version}")

    forbidden = [os.environ.get("ASANA_ACCESS_TOKEN", ""), str(args.dist.resolve())]
    runner_temp = os.environ.get("RUNNER_TEMP", "")
    if runner_temp:
        forbidden.append(runner_temp)
    sbom_names: list[str] = []
    for target, archive_name in sorted(archives.items()):
        match = ARCHIVE_RE.fullmatch(archive_name)
        assert match is not None
        sbom_name = f"{archive_name[:-len('.tar.gz')]}.sbom.spdx.json"
        output = args.dist / sbom_name
        if output.exists():
            output.unlink()
        result = subprocess.run(
            [args.tool, str(args.dist / archive_name), "-o", f"spdx-json={output}"],
            check=False,
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            fail(f"{args.tool} could not scan {archive_name}: {result.stderr.strip()}")
        if not output.is_file():
            fail(f"SBOM was not created for {archive_name}")
        validate_sbom(output, args.dist / archive_name, forbidden)
        sbom_names.append(sbom_name)

    update_checksums(args.dist, list(archives.values()) + sbom_names)
    print("generated and checksum-covered four SPDX release SBOMs")


if __name__ == "__main__":
    main()
