#!/usr/bin/env python3
"""Verify the public GitHub release, not just the runner's dist directory."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import stat
import subprocess
import sys
import tarfile
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


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


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("tag")
    parser.add_argument("--repository", default=os.environ.get("GITHUB_REPOSITORY", "thaodangspace/asana-cli"))
    args = parser.parse_args()
    token = os.environ.get("GITHUB_TOKEN", "")
    base = f"https://api.github.com/repos/{args.repository}"
    try:
        release = json.loads(request(f"{base}/releases/tags/{urllib.parse.quote(args.tag, safe='')}", token))
    except json.JSONDecodeError:
        fail("release response was invalid JSON")
    expected_prerelease = bool(re.search(r"-(?:rc|beta)\.", args.tag))
    if release.get("tag_name") != args.tag or bool(release.get("prerelease")) != expected_prerelease:
        fail("release tag or prerelease classification is incorrect")
    assets = {asset.get("name"): asset for asset in release.get("assets", []) if isinstance(asset, dict)}
    required = {"checksums.txt", "release-manifest.json"}
    required.update(name for name in assets if name.startswith("asana-cli_") and name.endswith(".tar.gz"))
    if len([name for name in required if name.startswith("asana-cli_")]) != 4 or not {"checksums.txt", "release-manifest.json"} <= assets.keys():
        fail("release does not contain exactly the expected four archives plus metadata")
    with tempfile.TemporaryDirectory() as temporary:
        directory = Path(temporary)
        downloaded: dict[str, bytes] = {}
        for name in required:
            asset = assets.get(name)
            if not asset or not isinstance(asset.get("id"), int):
                fail(f"release asset is missing: {name}")
            downloaded[name] = request(f"{base}/releases/assets/{asset['id']}", token, "application/octet-stream")
            (directory / name).write_bytes(downloaded[name])
        try:
            manifest = json.loads(downloaded["release-manifest.json"])
        except json.JSONDecodeError:
            fail("published release manifest is invalid JSON")
        if manifest.get("tag") != args.tag or manifest.get("version") != args.tag[1:]:
            fail("published manifest has the wrong tag or version")
        checksum_map = {}
        for line in downloaded["checksums.txt"].decode().splitlines():
            fields = line.split()
            if len(fields) == 2:
                checksum_map[fields[1]] = fields[0]
        archive_names = []
        targets = set()
        for artifact in manifest.get("artifacts", []):
            name = artifact.get("name")
            target = (artifact.get("goos"), artifact.get("goarch"))
            archive_names.append(name)
            targets.add(target)
            if name not in downloaded or digest(downloaded[name]) != artifact.get("sha256") or checksum_map.get(name) != artifact.get("sha256"):
                fail(f"published checksum mismatch for {name}")
        if len(archive_names) != 4 or targets != {("darwin", "amd64"), ("darwin", "arm64"), ("linux", "amd64"), ("linux", "arm64")}:
            fail("published manifest does not contain exactly the four supported targets")
        linux_name = next((item["name"] for item in manifest["artifacts"] if item["goos"] == "linux" and item["goarch"] == "amd64"), None)
        if not linux_name:
            fail("published manifest has no Linux amd64 archive")
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
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with Path(summary).open("a", encoding="utf-8") as handle:
            handle.write(f"## Public release verification\n\n- **Tag:** `{args.tag}`\n- **Result:** archive, checksum, version, and help checks passed\n- **Recovery:** `gh workflow run release.yml -f tag={args.tag} -f stage=verify`\n")
    print(f"public release verified: {args.tag}")


if __name__ == "__main__":
    main()
