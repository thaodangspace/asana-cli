#!/usr/bin/env python3
"""Promote a published release into the Homebrew tap, safely and independently."""

from __future__ import annotations

import argparse
import base64
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


def fail(message: str) -> "NoReturn":
    print(f"Homebrew promotion failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("formula", type=Path)
    parser.add_argument("version")
    parser.add_argument("--repository", default="thaodangspace/homebrew-tap")
    parser.add_argument("--branch", default="main")
    args = parser.parse_args()
    token = os.environ.get("TAP_GITHUB_TOKEN", "")
    if not token:
        fail("TAP_GITHUB_TOKEN is unavailable")
    if not args.formula.is_file():
        fail(f"formula is missing: {args.formula}")
    generated = args.formula.read_text(encoding="utf-8")
    api = f"https://api.github.com/repos/{args.repository}/contents/Formula/asana-cli.rb"
    headers = {"Accept": "application/vnd.github+json", "Authorization": f"Bearer {token}", "X-GitHub-Api-Version": "2022-11-28", "User-Agent": "asana-cli-release"}
    get_request = urllib.request.Request(api + "?" + urllib.parse.urlencode({"ref": args.branch}), headers=headers)
    existing = None
    try:
        with urllib.request.urlopen(get_request) as response:
            existing = __import__("json").loads(response.read())
    except urllib.error.HTTPError as exc:
        if exc.code != 404:
            fail(f"tap lookup returned HTTP {exc.code}")
    except (urllib.error.URLError, ValueError) as exc:
        fail(f"tap lookup failed: {exc}")

    current = ""
    current_sha = None
    if isinstance(existing, dict) and existing.get("content"):
        try:
            current = base64.b64decode(existing["content"]).decode("utf-8")
            current_sha = existing.get("sha")
        except (ValueError, UnicodeDecodeError):
            fail("tap formula is not valid UTF-8")
    if current:
        current_version = re.search(r'^\s*version "([^"]+)"$', current, re.MULTILINE)
        current_hashes = sorted(re.findall(r'^\s*sha256 "([0-9a-f]{64})"$', current, re.MULTILINE))
        generated_hashes = sorted(re.findall(r'^\s*sha256 "([0-9a-f]{64})"$', generated, re.MULTILINE))
        if current_version and current_version.group(1) == args.version:
            if current_hashes == generated_hashes:
                link = f"https://github.com/{args.repository}/blob/{args.branch}/Formula/asana-cli.rb"
                print(f"Homebrew formula is already current: {link}")
                return
            fail("tap has the same version with different archive hashes")

    body = {"message": f"Brew formula update for asana-cli version {args.version}", "content": base64.b64encode(generated.encode()).decode(), "branch": args.branch}
    if current_sha:
        body["sha"] = current_sha
    request = urllib.request.Request(api, data=__import__("json").dumps(body).encode(), headers={**headers, "Content-Type": "application/json"}, method="PUT")
    try:
        with urllib.request.urlopen(request) as response:
            result = __import__("json").loads(response.read())
    except urllib.error.HTTPError as exc:
        fail(f"tap update returned HTTP {exc.code}")
    except (urllib.error.URLError, ValueError) as exc:
        fail(f"tap update failed: {exc}")
    commit = result.get("commit", {}) if isinstance(result, dict) else {}
    link = commit.get("html_url") or f"https://github.com/{args.repository}/commits/{args.branch}"
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with Path(summary).open("a", encoding="utf-8") as handle:
            run_id = os.environ.get("GITHUB_RUN_ID", "<build-run-id>")
            handle.write(f"## Homebrew promotion\n\n- **Formula:** {link}\n- **Version:** `{args.version}`\n- **Recovery:** `gh workflow run release.yml -f tag=v{args.version} -f stage=promote -f build_run_id={run_id}`\n")
    print(f"Homebrew formula promoted: {link}")


if __name__ == "__main__":
    main()
