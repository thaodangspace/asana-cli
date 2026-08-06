---
title: Releasing asana-cli
description: The maintainer procedure for normal releases, verification, recovery, and rollback.
---

# Releasing asana-cli

This runbook is the release contract. Releases are forward-only: once users
may have downloaded a version, never move its tag or replace an asset with
different bytes.

## Normal release

1. Confirm `main` is green and the intended changes are merged. Review the
   generated changelog and compatibility impact.
2. Select the next semantic version: increment major for breaking CLI/output
   changes, minor for compatible commands/features, and patch for fixes.
3. Create an annotated tag from the intended `main` commit and push it:

   ```sh
   git checkout main && git pull --ff-only
   git tag -a vX.Y.Z -m "asana-cli vX.Y.Z"
   git push origin vX.Y.Z
   ```

4. Monitor `release-preflight`, `release-build`, `attest-release`,
   `publish-github-release`, `promote-homebrew`, `release-verify`, and
   `homebrew-verify`. Only the attestation and publication stages use the
   protected `release` environment.
5. Download the public assets and validate their checksums, manifest, SBOMs,
   and provenance locally (see the verification section below).
6. Install the formula and check `asana-cli --version`:

   ```sh
   brew update
   brew upgrade asana-cli
   asana-cli --version
   brew test asana-cli
   ```

7. Record the release URL, tag, and verification result in the changelog or
   release announcement.

The build creates four archives, `checksums.txt`, a manifest, and one SPDX JSON
SBOM per target. The manifest is built after all bytes exist and records the
full commit, repository, Go and GoReleaser versions, creation time, filenames,
sizes, and SHA-256 values.

## Verify a release locally

Use the current release tag rather than assuming a particular version:

```sh
tag="$(gh release view --repo thaodangspace/asana-cli --json tagName --jq .tagName)"
mkdir -p "release-$tag"
gh release download "$tag" --repo thaodangspace/asana-cli --dir "release-$tag"
cd "release-$tag"
sha256sum -c checksums.txt --ignore-missing
jq empty release-manifest.json
jq -e '.artifacts | map(select(.kind == "sbom")) | length == 4' release-manifest.json
for asset in asana-cli_*.tar.gz checksums.txt release-manifest.json *.sbom.spdx.json; do
  gh attestation verify "$asset" \
    --repo thaodangspace/asana-cli \
    --signer-workflow thaodangspace/asana-cli/.github/workflows/release.yml \
    --source-ref "refs/tags/$tag"
done
```

The SBOM filename is `asana-cli_X.Y.Z_<os>_<arch>.sbom.spdx.json`. Inspect it
with an SPDX-aware tool before ingesting it. Report a checksum, provenance, or
formula mismatch to the maintainers with the tag, expected digest, observed
digest, and command output; do not paste credentials.

## Prereleases

Accepted tags are `vX.Y.Z-rc.N` and `vX.Y.Z-beta.N`. GitHub marks these
releases as prereleases. They use the same four archives, manifest, SBOM, and
attestation checks, but stable Homebrew installation is skipped. Do not publish
a prerelease into the stable formula. Promote by creating a new stable tag
from the intended commit; never reuse or move a release-candidate tag.

## Hotfixes

A hotfix normally branches from the latest `main`, fixes the issue, and follows
all normal CI and release-preflight checks. If policy permits a hotfix branch
from the last release, merge the fix into `main` immediately afterward so it
cannot be lost. Select a new patch version and create a new tag; do not rebuild
or move the old tag.

## Retry and recovery

Retries reuse the immutable `release-build` artifact. Find the successful build
run ID in the Actions UI, then run the workflow manually:

```sh
gh workflow run release.yml -f tag=vX.Y.Z -f stage=publish -f build_run_id=RUN_ID
gh workflow run release.yml -f tag=vX.Y.Z -f stage=promote -f build_run_id=RUN_ID
gh workflow run release.yml -f tag=vX.Y.Z -f stage=verify -f build_run_id=RUN_ID
```

| Failure stage | Expected state | Safe recovery |
| --- | --- | --- |
| preflight | No external side effects | Fix source/config and create a new tag only if the original tag was never published. |
| build | No GitHub Release or tap change | Rerun infrastructure failures; source changes require a new version. |
| GitHub publish | Draft or partial assets may exist | Compare digests and resume identical uploads; never overwrite conflicts. |
| Homebrew promotion | GitHub Release is valid; formula is stale | Rerun promotion from the existing manifest/assets; do not rebuild. |
| attestation | Assets may exist without certified provenance | Attest the identical bytes and block declaring the release complete. |
| public verification | Release exists but is uncertified | Investigate without moving the tag; publish a corrected new version if bytes are wrong. |

If an existing release contains a conflicting asset, stop. Do not delete and
re-upload it as a shortcut. A malformed tag is corrected before publication
under the tag ruleset; a consumed tag is corrected with a new version.

## Rollback and secrets

Rollback means a forward correction. Do not delete or move a consumed tag and
do not replace assets under an existing version. Mark a broken release clearly
if needed, revert Homebrew to the last known-good version, and publish a new
patch version. Security-sensitive takedowns are an exceptional incident
process and must be documented separately.

`TAP_GITHUB_TOKEN` is a fine-grained token scoped only to the Homebrew tap with
minimum contents permission. Store it as a repository/environment secret, never
log it, and never reuse it for another repository. To rotate it, create the
replacement with the same minimum scope, update the secret, run a promotion
dry-run or a release recovery, then revoke the old token and record the date.
After rotation, confirm the promotion job can read/write only the tap and that
no workflow log contains the value. The release workflow does not require a
long-lived signing key; GitHub OIDC-backed attestations provide provenance.
