#!/usr/bin/env python3
"""Verify public release bytes, SBOMs, and GitHub build provenance."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

TARGETS = {("darwin", "amd64"), ("darwin", "arm64"), ("linux", "amd64"), ("linux", "arm64")}
ARCHIVE_RE = re.compile(r"^asana-cli_.+_(?:darwin|linux)_(?:amd64|arm64)\.tar\.gz$")
SBOM_RE = re.compile(r"^asana-cli_.+_(?:darwin|linux)_(?:amd64|arm64)\.sbom\.spdx\.json$")


def fail(message: str) -> "NoReturn":
    print(f"public release verification failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def request(url: str, token: str, accept: str = "application/vnd.github+json") -> bytes:
    headers = {"Accept": accept, "Authorization": f"Bearer {token}", "X-GitHub-Api-Version": "2022-11-28", "User-Agent": "asana-cli-release"}
    try:
        with urllib.request.urlopen(urllib.request.Request(url, headers=headers)) as response:
            return response.read()
    except urllib.error.HTTPError as exc:
        fail(f"public release request returned HTTP {exc.code}")
    except urllib.error.URLError as exc:
        fail(f"public release request failed: {exc.reason}")


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def verify_spdx(name: str, data: bytes) -> None:
    try:
        document = json.loads(data)
    except json.JSONDecodeError:
        fail(f"published SBOM is malformed JSON: {name}")
    if not isinstance(document, dict) or document.get("spdxVersion") not in {"SPDX-2.2", "SPDX-2.3"}:
        fail(f"published SBOM is not SPDX JSON: {name}")
    for key in ("SPDXID", "name", "documentNamespace", "creationInfo", "packages"):
        if key not in document:
            fail(f"published SBOM is missing {key}: {name}")


def verify_attestation(path: Path, repository: str, tag: str) -> None:
    if not shutil.which("gh"):
        fail("gh CLI is required for provenance verification")
    token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN")
    if not token:
        fail("GITHUB_TOKEN is unavailable for provenance verification")
    env = os.environ.copy()
    env["GH_TOKEN"] = token
    command = [
        "gh", "attestation", "verify", str(path),
        "--repo", repository,
        "--signer-workflow", f"{repository}/.github/workflows/release.yml",
        "--source-ref", f"refs/tags/{tag}",
    ]
    result = subprocess.run(command, check=False, capture_output=True, text=True, env=env)
    if result.returncode:
        fail(f"provenance verification failed for {path.name}")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("tag")
    parser.add_argument("--repository", default=os.environ.get("GITHUB_REPOSITORY", "thaodangspace/asana-cli"))
    args = parser.parse_args()
    token = os.environ.get("GITHUB_TOKEN", "")
    if not token:
        fail("GITHUB_TOKEN is unavailable")
    base = f"https://api.github.com/repos/{args.repository}"
    try:
        release = json.loads(request(f"{base}/releases/tags/{urllib.parse.quote(args.tag, safe='')}", token))
    except json.JSONDecodeError:
        fail("release response was invalid JSON")
    expected_prerelease = bool(re.search(r"-(?:rc|beta)\.", args.tag))
    if release.get("tag_name") != args.tag or bool(release.get("prerelease")) != expected_prerelease:
        fail("release tag or prerelease classification is incorrect")
    assets = {asset.get("name"): asset for asset in release.get("assets", []) if isinstance(asset, dict) and isinstance(asset.get("name"), str)}
    archive_names = sorted(name for name in assets if ARCHIVE_RE.fullmatch(name))
    sbom_names = sorted(name for name in assets if SBOM_RE.fullmatch(name))
    if len(archive_names) != 4 or len(sbom_names) != 4 or not {"checksums.txt", "release-manifest.json"} <= assets.keys():
        fail("release does not contain exactly four archives, four SBOMs, and metadata")
    expected_assets = set(archive_names + sbom_names + ["checksums.txt", "release-manifest.json"])
    if set(assets) != expected_assets:
        fail("release contains an unexpected asset")

    with tempfile.TemporaryDirectory() as temporary:
        directory = Path(temporary)
        downloaded: dict[str, bytes] = {}
        for name in sorted(expected_assets):
            asset = assets[name]
            if not isinstance(asset.get("id"), int):
                fail(f"release asset is missing an id: {name}")
            downloaded[name] = request(f"{base}/releases/assets/{asset['id']}", token, "application/octet-stream")
            (directory / name).write_bytes(downloaded[name])
        try:
            manifest = json.loads(downloaded["release-manifest.json"])
        except json.JSONDecodeError:
            fail("published release manifest is invalid JSON")
        if not isinstance(manifest, dict) or manifest.get("schema_version") != 1:
            fail("published manifest has an unsupported schema")
        if manifest.get("tag") != args.tag or manifest.get("version") != args.tag[1:]:
            fail("published manifest has the wrong tag or version")
        if manifest.get("source_repository") != args.repository or not re.fullmatch(r"[0-9a-f]{40}", str(manifest.get("commit", ""))):
            fail("published manifest has invalid source identity metadata")

        entries = manifest.get("artifacts")
        if not isinstance(entries, list) or len(entries) != 9:
            fail("published manifest does not contain four archives, four SBOMs, and checksums")
        checksum_map: dict[str, str] = {}
        for line in downloaded["checksums.txt"].decode("utf-8").splitlines():
            fields = line.split()
            if len(fields) == 2:
                checksum_map[fields[1]] = fields[0]
        archive_targets: set[tuple[str, str]] = set()
        sbom_targets: set[tuple[str, str]] = set()
        manifest_names: set[str] = set()
        for artifact in entries:
            if not isinstance(artifact, dict):
                fail("published manifest contains an invalid artifact")
            name = artifact.get("filename", artifact.get("name"))
            if not isinstance(name, str) or name not in downloaded or name in manifest_names:
                fail(f"published manifest references an invalid asset: {name}")
            manifest_names.add(name)
            data = downloaded[name]
            if artifact.get("size") != len(data) or artifact.get("sha256") != digest(data):
                fail(f"published digest or size mismatch for {name}")
            kind = artifact.get("kind")
            if kind == "archive":
                target = (artifact.get("os", artifact.get("goos")), artifact.get("arch", artifact.get("goarch")))
                if target not in TARGETS or target in archive_targets:
                    fail("published manifest has invalid archive targets")
                archive_targets.add(target)
                if checksum_map.get(name) != artifact.get("sha256"):
                    fail(f"checksums.txt disagrees with the manifest for {name}")
            elif kind == "sbom":
                target = (artifact.get("os"), artifact.get("arch"))
                if target not in TARGETS or target in sbom_targets:
                    fail("published manifest has invalid SBOM targets")
                sbom_targets.add(target)
                verify_spdx(name, data)
                if checksum_map.get(name) != artifact.get("sha256"):
                    fail(f"checksums.txt disagrees with the manifest for {name}")
            elif kind == "checksum" and name == "checksums.txt":
                if manifest.get("checksums", {}).get("sha256") != artifact.get("sha256"):
                    fail("published manifest checksum metadata disagrees with checksums.txt")
            else:
                fail(f"published manifest has an unknown artifact kind: {name}")
        if archive_targets != TARGETS or sbom_targets != TARGETS or manifest_names != expected_assets:
            fail("published manifest does not describe exactly the public assets")

        linux_name = next(item["filename"] for item in entries if item.get("kind") == "archive" and item.get("os", item.get("goos")) == "linux" and item.get("arch", item.get("goarch")) == "amd64")
        with tarfile.open(directory / linux_name, "r:gz") as archive:
            members = archive.getmembers()
            if not members or set(member.name for member in members) - {"asana-cli", "LICENSE", "README.md"} or sum(member.name == "asana-cli" for member in members) != 1:
                fail("published Linux amd64 archive contains unexpected files")
            binary_member = next(member for member in members if member.name == "asana-cli")
            if not binary_member.isfile():
                fail("published Linux amd64 binary is not a regular file")
            archive.extract(binary_member, directory)
        binary = directory / "asana-cli"
        binary.chmod(binary.stat().st_mode | stat.S_IXUSR)
        result = subprocess.run([str(binary), "--version"], check=False, capture_output=True, text=True)
        output = (result.stdout + result.stderr).strip()
        if result.returncode or not re.search(rf"(?<![0-9A-Za-z]){re.escape(args.tag[1:])}(?![0-9A-Za-z])", output):
            fail(f"published Linux binary reports the wrong version: {output}")
        if subprocess.run([str(binary), "--help"], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode:
            fail("published Linux binary --help failed")
        for name in sorted(expected_assets):
            verify_attestation(directory / name, args.repository, args.tag)

    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with Path(summary).open("a", encoding="utf-8") as handle:
            handle.write(f"## Public release verification\n\n- **Tag:** `{args.tag}`\n- **Result:** archives, checksums, SBOMs, version, help, and provenance checks passed\n- **Recovery:** `gh workflow run release.yml -f tag={args.tag} -f stage=verify`\n")
    print(f"public release, SBOM, and provenance verified: {args.tag}")


if __name__ == "__main__":
    main()
