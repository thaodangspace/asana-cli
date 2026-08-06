#!/usr/bin/env python3
"""Publish already-built release bytes with digest-safe, retryable semantics."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ASSETS = ("linux", "darwin")


def fail(message: str) -> "NoReturn":
    print(f"GitHub release publication failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


class GitHub:
    def __init__(self, repository: str, token: str) -> None:
        if not token:
            fail("GITHUB_TOKEN is unavailable")
        self.base = f"https://api.github.com/repos/{repository}"
        self.token = token

    def request(self, method: str, path: str, body: object | None = None, accept: str = "application/vnd.github+json") -> tuple[int, bytes, dict[str, str]]:
        data = None if body is None else json.dumps(body).encode()
        headers = {"Accept": accept, "Authorization": f"Bearer {self.token}", "X-GitHub-Api-Version": "2022-11-28", "User-Agent": "asana-cli-release"}
        if data is not None:
            headers["Content-Type"] = "application/json"
        request = urllib.request.Request(self.base + path, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(request) as response:
                return response.status, response.read(), dict(response.headers)
        except urllib.error.HTTPError as exc:
            if exc.code == 404:
                return 404, b"", dict(exc.headers)
            fail(f"GitHub API {method} {path} returned HTTP {exc.code}")
        except urllib.error.URLError as exc:
            fail(f"GitHub API request failed: {exc.reason}")

    def json(self, method: str, path: str, body: object | None = None) -> dict:
        status, data, _ = self.request(method, path, body)
        if not 200 <= status < 300:
            fail(f"GitHub API {method} {path} returned HTTP {status}")
        try:
            value = json.loads(data)
        except json.JSONDecodeError:
            fail(f"GitHub API {method} {path} returned invalid JSON")
        if not isinstance(value, dict):
            fail(f"GitHub API {method} {path} returned an unexpected response")
        return value


def manifest_assets(dist: Path) -> list[Path]:
    try:
        manifest = json.loads((dist / "release-manifest.json").read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read release manifest: {exc}")
    names = [item["name"] for item in manifest.get("artifacts", [])]
    names += ["checksums.txt", "release-manifest.json"]
    paths = [dist / name for name in names]
    if len(names) != 6 or any(not path.is_file() for path in paths):
        fail("build artifact is missing one or more release assets")
    return paths


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("dist", type=Path)
    parser.add_argument("tag")
    parser.add_argument("--repository", default=os.environ.get("GITHUB_REPOSITORY", "thaodangspace/asana-cli"))
    parser.add_argument("--output", default=os.environ.get("GITHUB_OUTPUT"))
    args = parser.parse_args()
    dist = args.dist
    manifest = json.loads((dist / "release-manifest.json").read_text(encoding="utf-8"))
    if manifest.get("tag") != args.tag:
        fail("manifest tag does not match the requested release tag")
    paths = manifest_assets(dist)
    github = GitHub(args.repository, os.environ.get("GITHUB_TOKEN", ""))
    encoded_tag = urllib.parse.quote(args.tag, safe="")
    status, data, _ = github.request("GET", f"/releases/tags/{encoded_tag}")
    if status == 404:
        release = github.json("POST", "/releases", {"tag_name": args.tag, "target_commitish": manifest["commit"], "name": args.tag, "generate_release_notes": True, "prerelease": bool(manifest.get("prerelease"))})
    else:
        try:
            release = json.loads(data)
        except json.JSONDecodeError:
            fail("existing release response was invalid JSON")
        if not isinstance(release, dict) or release.get("tag_name") != args.tag:
            fail("existing release has an unexpected tag")
        if bool(release.get("prerelease")) != bool(manifest.get("prerelease")):
            fail("existing release has the wrong prerelease classification")
        target = release.get("target_commitish")
        if isinstance(target, str) and len(target) == 40 and target != manifest.get("commit"):
            fail("existing release points at a different commit")
        # Do not recreate or replace an existing release. Completing a draft is safe,
        # while its existing assets are still checked byte-for-byte below.
        if release.get("draft"):
            release = github.json("PATCH", f"/releases/{release['id']}", {"draft": False, "prerelease": bool(manifest.get("prerelease"))})

    release_id = release.get("id")
    upload_url = release.get("upload_url")
    if not isinstance(release_id, int) or not isinstance(upload_url, str):
        fail("release response has no id or upload URL")
    upload_url = upload_url.split("{", 1)[0]
    existing = {asset.get("name"): asset for asset in release.get("assets", []) if isinstance(asset, dict)}
    expected_names = {path.name for path in paths}
    unexpected = sorted(name for name in existing if name not in expected_names)
    if unexpected:
        fail(f"existing release contains unexpected assets: {', '.join(unexpected)}")
    published: list[str] = []
    for path in paths:
        name = path.name
        expected = sha256_bytes(path.read_bytes())
        asset = existing.get(name)
        if asset:
            asset_id = asset.get("id")
            if not isinstance(asset_id, int):
                fail(f"existing asset {name} has no id")
            status, content, _ = github.request("GET", f"/releases/assets/{asset_id}", accept="application/octet-stream")
            if status != 200 or sha256_bytes(content) != expected:
                fail(f"existing asset {name} has a conflicting digest")
            published.append(name)
            continue
        request = urllib.request.Request(upload_url + "?" + urllib.parse.urlencode({"name": name}), data=path.read_bytes(), headers={"Accept": "application/vnd.github+json", "Authorization": f"Bearer {github.token}", "Content-Type": "application/octet-stream", "User-Agent": "asana-cli-release"}, method="POST")
        try:
            with urllib.request.urlopen(request) as response:
                if not 200 <= response.status < 300:
                    fail(f"upload of {name} returned HTTP {response.status}")
        except urllib.error.HTTPError as exc:
            fail(f"upload of {name} returned HTTP {exc.code}")
        except urllib.error.URLError as exc:
            fail(f"upload of {name} failed: {exc.reason}")
        published.append(name)

    url = release.get("html_url")
    if not isinstance(url, str):
        fail("release response has no URL")
    output = f"release_id={release_id}\nrelease_url={url}\n"
    if args.output:
        Path(args.output).open("a", encoding="utf-8").write(output)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with Path(summary).open("a", encoding="utf-8") as handle:
            run_id = os.environ.get("GITHUB_RUN_ID", "<build-run-id>")
            handle.write(f"## GitHub Release\n\n- **Tag:** `{args.tag}`\n- **URL:** {url}\n- **Assets:** {', '.join(published)}\n- **Recovery:** `gh workflow run release.yml -f tag={args.tag} -f stage=publish -f build_run_id={run_id}`\n")
    print(f"published {len(published)} release assets: {url}")


if __name__ == "__main__":
    main()
