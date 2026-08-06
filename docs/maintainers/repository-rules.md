---
title: Repository rules and release boundaries
description: Branch, tag, environment, and dependency maintenance policy.
---

# Repository rules and release boundaries

This page is the declarative record for settings that are not reviewable as
files in a pull request. Apply the settings in **Settings → Rules → Rulesets**
and keep the exported ruleset JSON beside this document when GitHub changes it.
The repository owner is responsible for reviewing the live settings after each
change.

## `main` branch ruleset

The `main` ruleset is required to be enforced for everyone, including
administrators during normal development. Changes merge through pull requests;
force-pushes and branch deletion are blocked. Require these check names, exactly
as emitted by CI:

- `go-quality`
- `build-smoke`
- `cross-build`
- `docs-build`
- `release-snapshot`

Require branches to be up to date where the GitHub ruleset supports that
option, and require conversations to be resolved before merge. Check names are
a compatibility contract: rename a job only with a coordinated ruleset change.
An emergency administrator bypass must be recorded in the pull request or an
incident note with the reason and follow-up.

## Release tag ruleset

Apply a second ruleset to `v*` tags. Restrict tag creation to maintainers or the
release automation path, and block deletion and force-updates. A malformed tag
is handled before publication according to the runbook; a published tag is
never moved to repair a failed release.

## `release` environment

Create a protected environment named `release`. Only the attestation and
GitHub Release publication jobs may reference it. Configure any approval rule
explicitly and restrict deployments to release tags (`v*`). Keep environment
secrets limited to release credentials. `TAP_GITHUB_TOKEN` is used only by the
Homebrew promotion job and should be stored as a repository or environment
secret with the minimum contents permission on the tap repository.

The workflow intentionally scopes `id-token: write` and `attestations: write` to
`attest-release`; PR, docs, snapshot, and Homebrew jobs do not receive those
permissions.

## Applying settings

After applying settings, export the rulesets with the GitHub CLI and attach the
result to the settings change or update this page:

```sh
gh api repos/thaodangspace/asana-cli/rulesets > /tmp/asana-cli-rulesets.json
gh api repos/thaodangspace/asana-cli/environments/release > /tmp/asana-cli-release-environment.json
```

Check that the live rulesets still include the five check names, immutable
`v*` tags, and no unrelated bypass actors. Never put token values in the
export, issue, pull request, or this repository.

## Dependency update policy

Dependabot runs weekly updates for Go modules, the docs npm lockfile, and
GitHub Actions. Reviewers are the maintainers who own the affected area. A
patch/minor PR is acceptable when the lockfile/module diff is expected and all
required checks pass. Major updates remain separate migration PRs and must
include release-snapshot/formula validation plus any required user-facing
migration notes. Dependency PRs are never auto-merged: #33 CI and #34 release
snapshot checks must be required before considering an update.

GitHub Actions updates must retain immutable full-SHA references and the
readable version comment. Go updates must include `go mod tidy` output in
`go.mod`/`go.sum`; docs updates must pass `npm ci` from the committed lockfile.
GoReleaser and release-tool updates additionally require a snapshot artifact
comparison and formula-generation validation.
