# asana-cli

A standalone Go CLI for Asana, ported from the `pi-extensions` Asana extension so
any agent (or human/script) can invoke Asana operations from the shell. Output is
**JSON by default** for deterministic machine parsing; pass `--human` for readable
summaries.

## Install

### Homebrew (recommended)

```bash
brew tap dtonair/tap
brew install asana-cli
```

To upgrade later: `brew upgrade asana-cli`.

### go install

```bash
go install github.com/dtonair/asana-cli/cmd/asana-cli@latest
```

This installs the `asana-cli` binary into `$(go env GOBIN)` (or `$(go env
GOPATH)/bin`); make sure that directory is on your `PATH`.

### Build from source

```bash
go build -o asana-cli ./cmd/asana-cli
# or
go install ./cmd/asana-cli
```

Requires Go 1.22+. The only third-party dependency is `spf13/cobra`.

Check the installed version with `asana-cli --version`.

## Configuration

Credentials come from environment variables or a YAML config file. Environment
variables take precedence over the file.

### Environment variables

```bash
export ASANA_ACCESS_TOKEN="your-asana-personal-access-token"   # required
export ASANA_DEFAULT_WORKSPACE="workspace-gid"                 # optional
```

### Config file

If `ASANA_ACCESS_TOKEN` is not set, the CLI reads `~/.config/asana-cli.yaml`
(override the path with `$ASANA_CONFIG`, or relocate via `$XDG_CONFIG_HOME`):

```yaml
# ~/.config/asana-cli.yaml
access_token: your-asana-personal-access-token   # required
default_workspace: workspace-gid                  # optional
```

Each value is used only when the corresponding env var is unset, so you can keep
a token in the file and still override it per-shell with `ASANA_ACCESS_TOKEN`.
Keep the file private (`chmod 600 ~/.config/asana-cli.yaml`).

`default_workspace`/`ASANA_DEFAULT_WORKSPACE` is optional, but workspace-scoped
commands require either it or an explicit `--workspace-gid`.

Recommended token scopes: `users:read`, `workspaces:read`, `projects:read`,
`tasks:read`, `stories:read`, `attachments:read`, `attachments:write` for
`add-attachment`, `stories:write` for `comment-on-task`, and `tasks:write` for
`update-task`.

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--human` | off | Print human-readable summaries instead of JSON |
| `--verbose` | off | Log request method + path to stderr (never the token) |
| `--timeout` | `30s` | HTTP request timeout |

## Commands

| Command | Asana endpoint | Notes |
|---------|----------------|-------|
| `me` | `GET /users/me` | |
| `list-workspaces` | `GET /workspaces` | `--limit`, `--all`, `--offset`, `--max-pages`, `--opt-fields` |
| `list-projects` | `GET /workspaces/{ws}/projects` | `--workspace-gid`, pagination flags, `--opt-fields` |
| `list-project-tasks` | `GET /projects/{project_gid}/tasks` | `--project-gid`, pagination flags, `--opt-fields` |
| `list-tag-tasks` | `GET /tags/{tag_gid}/tasks` | `--tag-gid`, pagination flags, `--opt-fields` |
| `search-tasks` | `GET /workspaces/{ws}/tasks/search` | search filters, pagination flags, `--opt-fields` (may require premium) |
| `get-task` | `GET /tasks/{gid}` | `--task-gid` (required), `--opt-fields` |
| `list-task-stories` | `GET /tasks/{gid}/stories` | `--task-gid` (required), pagination flags, `--opt-fields` |
| `list-task-attachments` | `GET /attachments?parent={task_gid}` | `--task-gid` (required), pagination flags, `--opt-fields` |
| `get-attachment` | `GET /attachments/{gid}` | `--attachment-gid` (required), `--opt-fields` |
| `download-attachment` | `GET /attachments/{gid}` then attachment `download_url` | `--attachment-gid`, `--output` (both required), `--overwrite` |
| `add-attachment` | `POST /attachments` (multipart) | `--task-gid`, `--file` (both required), `--name`. Write command. |
| `comment-on-task` | `POST /tasks/{gid}/stories` | `--task-gid`, `--text` (both required). Write command. |
| `update-task` | `PUT /tasks/{gid}` | `--task-gid` (required) plus ≥1 of `--name`, `--notes`, `--completed`, `--due-on`, `--assignee`. Write command. |

All list/search commands support `--limit` (the maximum number of items),
`--all`, `--offset`, and `--max-pages`. `--all` follows pages until Asana
reports the collection is exhausted; it cannot be combined with an explicitly
provided `--limit`. By default, existing limits and the ten-page safety bound
are retained. `--all --max-pages N` provides a bounded full traversal.
`--offset` resumes from an Asana offset token. `--max-pages` must be positive.

Requests use a page size of 50. `--completed` is tri-state: omitted entirely
unless you pass it (`--completed=true` or `--completed=false`).

## Output contract

**Success** (stdout):

```json
{
  "ok": true,
  "data": <Asana resource (object) or array of resources>,
  "pagination": {
    "pages_fetched": 3,
    "truncated": true,
    "next_offset": "...",
    "next_path": "..."
  }
}
```

`data` is the unwrapped Asana payload: an object for single-resource commands
(`me`, `get-task`, `get-attachment`, `add-attachment`, `comment-on-task`,
`update-task`), an array for list/search commands, and a download result object
for `download-attachment`. List/search commands also include `pagination` with
`pages_fetched` and `truncated`; bounded results expose `next_offset` and/or
`next_path` when Asana provides a resumable next page.

**Error** (stderr, non-zero exit):

```json
{
  "ok": false,
  "error": {
    "message": "Asana resource not found. ...",
    "status": 404,
    "method": "GET",
    "path": "/tasks/999"
  }
}
```

`status`/`method`/`path` are present only for HTTP errors. With `--human`, errors
print as a plain message line instead.

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Runtime error (HTTP non-2xx, network failure, timeout) |
| `2` | Usage/config error (missing token, missing required flag, bad `--limit`, no workspace) |

## Examples

```bash
asana-cli me
asana-cli list-workspaces --limit 50
asana-cli list-workspaces --all
asana-cli list-workspaces --all --max-pages 20
asana-cli list-workspaces --offset eyJ0eXAiOiJKV1Qi...
asana-cli list-projects --workspace-gid 12345
asana-cli list-project-tasks --project-gid 12345
asana-cli list-tag-tasks --tag-gid 67890
asana-cli search-tasks --text "release" --completed=false
asana-cli get-task --task-gid 12345 --human
asana-cli list-task-stories --task-gid 12345
asana-cli list-task-attachments --task-gid 12345
asana-cli get-attachment --attachment-gid 67890 --opt-fields name,download_url
asana-cli download-attachment --attachment-gid 67890 --output ./Screenshot.png
asana-cli add-attachment --task-gid 12345 --file ./Screenshot.png
asana-cli add-attachment --task-gid 12345 --file ./out.log --name "run.log"
asana-cli comment-on-task --task-gid 12345 --text "Taking a look."
asana-cli update-task --task-gid 12345 --completed
asana-cli update-task --task-gid 12345 --name "Ship v2" --due-on 2026-07-15
asana-cli update-task --task-gid 12345 --assignee me --notes "Reassigned."
asana-cli update-task --task-gid 12345 --due-on ""   # clears the due date
```

## Documentation

The documentation site is an Astro/Starlight app under `docs/`. Run it locally
with `make docs-dev`, or build the static site with `make docs-build`. Cloudflare
Pages settings are documented in [`docs/README.md`](docs/README.md).

## Test

```bash
go test ./...
```

Tests run against an in-process `httptest` server; no network or real token is
required. The API base URL is overridable via the `ASANA_API_BASE` environment
variable (test-only seam; not a user-facing flag).

## Security

The token is read from the environment or, as a fallback, from
`~/.config/asana-cli.yaml`. The CLI never writes the token to disk itself
(you create the config file) and never includes it in any rendered output,
error, or `--verbose` log line. If you store the token in the config file,
restrict its permissions (`chmod 600`). All API requests go over HTTPS to
`https://app.asana.com/api/1.0`.

`download-attachment` writes attachment bytes only to the file named by
`--output`. It refuses to overwrite existing files unless `--overwrite` is
provided and removes partial output files when a download fails.
