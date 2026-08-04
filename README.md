# asana-cli

A standalone Go CLI for Asana, ported from the `pi-extensions` Asana extension so
any agent (or human/script) can invoke Asana operations from the shell. Output is
**JSON by default** for deterministic machine parsing; pass `--human` for readable
summaries.

## Install

### Homebrew (recommended)

```bash
brew tap thaodangspace/tap
brew install asana-cli
```

To upgrade later: `brew upgrade asana-cli`.

### go install

```bash
go install github.com/thaodangspace/asana-cli/cmd/asana-cli@latest
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
`projects:write` for project/section lifecycle commands, `teams:read`,
`tasks:read`, `stories:read`, `attachments:read`, `attachments:write` for
`add-attachment`, `stories:write` for `comment-on-task`, and `tasks:write` for
`create-task`, `update-task`, `duplicate-task`, and `delete-task`.

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--human` | off | Print human-readable summaries instead of JSON |
| `--verbose` | off | Log request method + path to stderr (never the token) |
| `--timeout` | `30s` | HTTP request timeout, including retries |
| `--max-retries` | `3` | retries for transient GET/DELETE requests after the initial request |
| `--retry-max-wait` | `30s` | cap for one retry delay, including `Retry-After` |
| `--no-retry` | off | disable retries for deterministic one-shot behavior |

## Commands

| Command | Asana endpoint | Notes |
|---------|----------------|-------|
| `me` | `GET /users/me` | |
| `list-workspaces` | `GET /workspaces` | `--limit`, `--all`, `--offset`, `--max-pages`, `--opt-fields` |
| `list-projects` | `GET /workspaces/{ws}/projects` | `--workspace-gid`, pagination flags, `--opt-fields` |
| `get-project` | `GET /projects/{gid}` | `--project-gid`, `--opt-fields` |
| `create-project` | `POST /projects` | project fields, workspace/team context |
| `update-project` | `PUT /projects/{gid}` | `--project-gid` plus changed project fields |
| `delete-project` | `DELETE /projects/{gid}` | `--project-gid`, `--confirm` or `--yes` |
| `duplicate-project` | `POST /projects/{gid}/duplicate` | `--include` and repeatable `--option` |
| `search-projects` | `GET /workspaces/{ws}/projects/search` | one-page filters, `--limit`, repeatable `--query` |
| `list-team-projects` | `GET /teams/{gid}/projects` | `--team-gid`, pagination flags |
| `list-project-tasks` | `GET /projects/{project_gid}/tasks` | `--project-gid`, pagination flags, `--opt-fields` |
| `list-tag-tasks` | `GET /tags/{tag_gid}/tasks` | `--tag-gid`, pagination flags, `--opt-fields` |
| `search-tasks` | `GET /workspaces/{ws}/tasks/search` | search filters, one-page `--limit` (1-100), `--opt-fields` (may require premium) |
| `get-task` | `GET /tasks/{gid}` | `--task-gid` (required), `--opt-fields` |
| `list-task-stories` | `GET /tasks/{gid}/stories` | `--task-gid` (required), pagination flags, `--opt-fields` |
| `list-task-attachments` | `GET /attachments?parent={task_gid}` | `--task-gid` (required), pagination flags, `--opt-fields` |
| `get-attachment` | `GET /attachments/{gid}` | `--attachment-gid` (required), `--opt-fields` |
| `download-attachment` | `GET /attachments/{gid}` then attachment `download_url` | `--attachment-gid`, `--output` (both required), `--overwrite` |
| `add-attachment` | `POST /attachments` (multipart) | `--task-gid`, `--file` (both required), `--name`. Write command. |
| `comment-on-task` | `POST /tasks/{gid}/stories` | `--task-gid`, `--text` (both required). Write command. |
| `create-task` | `POST /tasks` | `--name` plus workspace, project, or parent context; supports dates, notes, followers, sections, and custom fields. |
| `update-task` | `PUT /tasks/{gid}` | `--task-gid` (required) plus ≥1 mutable field. Supports explicit clearing and tri-state booleans. |
| `delete-task` | `DELETE /tasks/{gid}` | `--task-gid` and `--confirm` or `--yes`. Returns `data: null`. |
| `duplicate-task` | `POST /tasks/{gid}/duplicate` | `--task-gid`, `--name`, and repeatable/comma-separated `--include` options. Returns an asynchronous job. |
| `list-sections` / `get-section` | `GET /projects/{gid}/sections`, `GET /sections/{gid}` | section/project GID |
| `create-section` / `update-section` | `POST /projects/{gid}/sections`, `PUT /sections/{gid}` | `--name` |
| `delete-section` / `move-section` | `DELETE /sections/{gid}`, `POST /projects/{gid}/sections/insert` | confirmation and before/after positioning |
| `add-task-to-section` | `POST /sections/{gid}/addTask` | task GID and optional before/after positioning |
| `list-section-tasks` | `GET /sections/{gid}/tasks` | section GID and pagination |
| `list-subtasks`, `list-dependencies`, `list-dependents` | `GET /tasks/{gid}/{relationship}` | `--task-gid`, pagination flags, and `--opt-fields`. |
| `create-subtask` | `POST /tasks/{gid}/subtasks` | `--task-gid`, `--name`, and common task fields. |
| `set-task-parent`, `remove-task-parent` | `POST /tasks/{gid}/setParent` | Set or clear a task parent. |
| `add-dependency`, `remove-dependency` | `POST /tasks/{gid}/{operation}` | `--task-gid` and `--dependency-task-gid`. |
| `add-task-to-project`, `remove-task-from-project` | `POST /tasks/{gid}/{operation}` | Project membership; add supports section and mutually exclusive positioning flags. |
| `add-tag-to-task`, `remove-tag-from-task` | `POST /tasks/{gid}/{operation}` | `--task-gid` and `--tag-gid`. |
| `add-task-followers`, `remove-task-followers` | `POST /tasks/{gid}/{operation}` | Repeatable ordered `--follower` user GIDs or `me`. |
| `api` | arbitrary relative API path | `METHOD PATH`, repeatable `--query`, optional JSON `--data`/`@FILE`, and `--raw-response`. Advanced escape hatch. |

List commands support `--limit` (the maximum number of items), `--all`,
`--offset`, and `--max-pages`. `--all` follows pages until Asana reports the
collection is exhausted; it cannot be combined with an explicitly provided
`--limit`. By default, existing limits and the ten-page safety bound are
retained. `--all --max-pages N` provides a bounded full traversal. `--offset`
resumes from an Asana offset token. `--max-pages` must be positive.

Collection requests use a page size of 50. `search-tasks` and `search-projects`
are exceptions: each accepts one request page with `--limit` from 1 through 100
and has no offset or continuation flags. It provides first-class text, assignee, project, section,
tag, team, follower, completion, due/start date, timestamp, subtype, and
sorting filters. Repeatable filters such as `--project-any` are joined as
comma-separated Asana query values. Use repeatable `--query key=value` for
custom-field and future search parameters; conflicting scalar keys are
rejected and `limit`/`offset` cannot be overridden. `--completed` is tri-state:
omitted entirely unless you pass it (`--completed=true` or
`--completed=false`). Task creation requires `--name` and a workspace, project,
or parent context. `--project-gid`, `--follower`, and `--custom-field` are repeatable. Custom field scalar values
are strings by default, preserving numeric Asana GIDs; prefix values with
`json:` for numbers, arrays, booleans, null, or quoted strings (for example
`--custom-field 123=json:["option-a","option-b"]`). Date-only flags use
`YYYY-MM-DD`; date-time flags use RFC 3339. A start date requires the matching
due date in the same invocation, and date/date-time variants cannot be mixed.
`--notes` and `--html-notes` are mutually exclusive. Update project, section,
and parent relationships use Asana's dedicated endpoints rather than PUT.

GET and DELETE requests retry `429`, `500`, `502`, `503`, and `504` responses,
as well as temporary network failures, up to three times by default. `Retry-After`
is honored when present (integer seconds or an HTTP date) and capped by
`--retry-max-wait`; other delays use exponential backoff with jitter. Retries
stop when `--timeout` or the command context is canceled. JSON POST/PUT requests
and multipart uploads are intentionally not retried because they may mutate
Asana more than once. Use `--no-retry` for one-shot behavior. With `--verbose`,
retry attempts are logged to stderr with only the method and safe path.

The advanced `api` command supports authenticated GET, POST, PUT, PATCH, and
DELETE requests to relative paths under the Asana API base:

```bash
asana-cli api GET /tasks/123
asana-cli api GET /workspaces/123/tasks/search --query 'projects.any=456' --query 'completed=false'
asana-cli api POST /tasks --data '{"data":{"name":"Ship v2","workspace":"123"}}'
asana-cli api PUT /tasks/123 --data @update.json
asana-cli api DELETE /tasks/123
```

`--data` must be valid JSON and supports files up to 10 MiB. Absolute or
cross-origin paths are rejected, and callers cannot override authorization
headers. Responses unwrap Asana's `data` field by default; `--raw-response`
keeps the complete decoded response envelope. Empty successful responses return
`data: null`.

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
`create-task`, `update-task`), a duplication job for `duplicate-task`, `null`
for `delete-task`, an array for list/search commands, and a download result object
for `download-attachment`. The advanced `api` command unwraps `data` by default
(or returns the complete response with `--raw-response`). List/search commands also include `pagination` with
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
asana-cli create-project --workspace-gid 12345 --name "Launch v2" --public
asana-cli create-section --project-gid 67890 --name "In progress"
asana-cli add-task-to-section --section-gid 999 --task-gid 123 --after-task-gid 122
asana-cli duplicate-project --project-gid 67890 --include tasks,members
asana-cli list-project-tasks --project-gid 12345
asana-cli list-tag-tasks --tag-gid 67890
asana-cli search-tasks --text "release" --completed=false
asana-cli search-tasks --project-any 123 --due-before 2026-08-31 --sort-by due_date
asana-cli search-tasks --query 'custom_fields.111.value=222' \
  --query 'modified_at.after=2026-08-01T00:00:00Z'
asana-cli get-task --task-gid 12345 --human
asana-cli list-task-stories --task-gid 12345
asana-cli list-task-attachments --task-gid 12345
asana-cli get-attachment --attachment-gid 67890 --opt-fields name,download_url
asana-cli download-attachment --attachment-gid 67890 --output ./Screenshot.png
asana-cli add-attachment --task-gid 12345 --file ./Screenshot.png
asana-cli add-attachment --task-gid 12345 --file ./out.log --name "run.log"
asana-cli comment-on-task --task-gid 12345 --text "Taking a look."
asana-cli create-task --workspace-gid 12345 --name "Ship v2" --project-gid 67890 --due-on 2026-07-15
asana-cli create-task --name "Subtask" --parent-task-gid 12345 --follower 67890
asana-cli update-task --task-gid 12345 --completed
asana-cli update-task --task-gid 12345 --name "Ship v2" --due-on 2026-07-15
asana-cli update-task --task-gid 12345 --assignee me --notes "Reassigned."
asana-cli delete-task --task-gid 12345 --yes
asana-cli duplicate-task --task-gid 12345 --name "Copy of Ship v2" --include subtasks,dependencies
asana-cli create-subtask --task-gid 12345 --name "Write release notes" --assignee me
asana-cli add-dependency --task-gid 12345 --dependency-task-gid 67890
asana-cli add-task-to-project --task-gid 12345 --project-gid 67890 --section-gid 999
asana-cli add-task-followers --task-gid 12345 --follower me --follower 67890
asana-cli update-task --task-gid 12345 --due-on ""   # clears the due date
asana-cli api GET /teams/123/memberships --query 'limit=50'
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
