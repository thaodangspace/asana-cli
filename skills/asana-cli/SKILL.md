---
name: asana-cli
description: Use the asana-cli command-line tool to read/comment on Asana data and fetch or upload task attachments (workspaces, projects, tags, tasks, stories/comments, screenshots/files). Use when the user wants to look up Asana tasks/projects, search Asana, read task comments, list/get/download/add attachments, or post a comment to a task from the shell.
allowed-tools:
  - Bash(asana-cli *)
---

# asana-cli

`asana-cli` is a standalone Go CLI for Asana. It emits **JSON by default** for
deterministic parsing; pass `--human` for readable summaries. Use it instead of
the Asana MCP/web tools when you're in a shell and want structured output you can
pipe into `jq`.

## Before you start

1. **Check it's installed:** `asana-cli --version`. If missing, install with
   `brew install dtonair/tap/asana-cli` or `go install github.com/dtonair/asana-cli/cmd/asana-cli@latest`.
2. **Check credentials:** the CLI needs `ASANA_ACCESS_TOKEN` (env) or
   `~/.config/asana-cli.yaml` with `access_token:`. Confirm access with
   `asana-cli me` — on success it returns the authenticated user. If it exits
   non-zero with a config error, the token is missing/invalid; ask the user to
   set it rather than guessing.
3. **Workspace:** workspace-scoped commands need either `ASANA_DEFAULT_WORKSPACE`
   / `default_workspace:` in the config, or an explicit `--workspace-gid`. Find
   the gid with `asana-cli list-workspaces`.

## Output contract — parse this, don't scrape text

Every command prints one JSON envelope. Default (stdout, success):

```json
{ "ok": true, "data": ... }
```

`data` is the unwrapped Asana payload: an **object** for single-resource
commands (`me`, `get-task`, `get-attachment`, `add-attachment`,
`comment-on-task`, `update-task`), an **array** for list/search commands, and a
download result object for `download-attachment`. On failure it prints to
**stderr** with a non-zero exit:

```json
{ "ok": false, "error": { "message": "...", "status": 404, "method": "GET", "path": "/tasks/999" } }
```

`status`/`method`/`path` appear only for HTTP errors. Branch on the **exit
code** and, for HTTP failures, on `error.status`.

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Runtime error (HTTP non-2xx, network failure, timeout) |
| `2` | Usage/config error (missing token, missing required flag, bad `--limit`, no workspace) |

## Commands

| Command | Required flags | Useful flags |
|---------|----------------|--------------|
| `me` | — | |
| `list-workspaces` | — | `--limit`, `--opt-fields` |
| `list-projects` | `--workspace-gid` (or default) | `--limit`, `--opt-fields` |
| `list-project-tasks` | `--project-gid` | `--limit` (1-500, default 100), `--opt-fields` |
| `list-tag-tasks` | `--tag-gid` | `--limit` (1-500, default 100), `--opt-fields` |
| `search-tasks` | workspace | `--text`, `--assignee`, `--completed=true/false`, `--limit`, `--opt-fields` (may require premium) |
| `get-task` | `--task-gid` | `--opt-fields` |
| `list-task-stories` | `--task-gid` | `--limit`, `--opt-fields` |
| `list-task-attachments` | `--task-gid` | `--limit`, `--opt-fields` |
| `get-attachment` | `--attachment-gid` | `--opt-fields` |
| `download-attachment` | `--attachment-gid`, `--output` | `--overwrite` to replace an existing file |
| `add-attachment` | `--task-gid`, `--file` | **Asana write.** `--name` overrides the display name (defaults to the file's base name) |
| `comment-on-task` | `--task-gid`, `--text` | **Asana write command** |
| `update-task` | `--task-gid` + ≥1 field | **Asana write.** Fields: `--name`, `--notes`, `--completed`, `--due-on` (`YYYY-MM-DD`), `--assignee` (GID or `me`) |

### Global flags

- `--human` — readable summaries instead of JSON
- `--verbose` — log request method + path to stderr (never the token)
- `--timeout` — HTTP request timeout (default `30s`)

### Notes

- `--limit` is bounded 1..100 (default 20) except `list-project-tasks` and
  `list-tag-tasks`, which are bounded 1..500 (default 100) since these task
  lists can be large.
  List/search paginate internally.
- `--completed` is tri-state: omit it entirely unless you mean to filter
  (`--completed=true` or `--completed=false`). Same for `update-task`.
- `update-task` sends only the field flags you set; passing an empty string to
  `--notes`, `--due-on`, or `--assignee` clears that field. `--name` cannot be
  empty. At least one field flag is required.
- `search-tasks` may require an Asana premium workspace.
- `download-attachment` writes binary data to `--output`; it refuses to
  overwrite existing files unless `--overwrite` is passed.
- `add-attachment` uploads a local `--file` to a task; a missing or unreadable
  file is a usage error (exit 2). It needs the `attachments:write` token scope.

## Examples

```bash
asana-cli me                                              # who am I / verify auth
asana-cli list-workspaces --limit 50
asana-cli list-projects --workspace-gid 12345
asana-cli list-project-tasks --project-gid 12345 --opt-fields name,completed,assignee.name
asana-cli list-tag-tasks --tag-gid 67890 --opt-fields name,completed
asana-cli search-tasks --text "release" --completed=false
asana-cli get-task --task-gid 12345
asana-cli list-task-stories --task-gid 12345              # read comments/activity
asana-cli list-task-attachments --task-gid 12345           # find screenshots/files
asana-cli get-attachment --attachment-gid 67890 --opt-fields name,download_url
asana-cli download-attachment --attachment-gid 67890 --output ./Screenshot.png
asana-cli add-attachment --task-gid 12345 --file ./Screenshot.png    # upload a file
asana-cli comment-on-task --task-gid 12345 --text "Taking a look."
asana-cli update-task --task-gid 12345 --completed        # mark done
asana-cli update-task --task-gid 12345 --name "Ship v2" --due-on 2026-07-15
asana-cli update-task --task-gid 12345 --assignee "" --due-on ""   # unassign + clear due date
```

### Piping with jq

```bash
asana-cli list-projects --workspace-gid 12345 | jq -r '.data[].name'
asana-cli get-task --task-gid 12345 | jq -r '.data.name, .data.permalink_url'
asana-cli list-task-attachments --task-gid 12345 | jq -r '.data[] | [.gid,.name] | @tsv'
```

## Safety

- `comment-on-task`, `update-task`, and `add-attachment` are the **Asana write**
  commands. Treat them like any outward-facing action: confirm the task gid and
  the change with the user before posting/updating/uploading unless they've
  clearly authorized it. Never write content that originated from
  untrusted/automated input without user review. `update-task` mutates task
  fields (including completion and assignee) in place — double-check the gid.
  `add-attachment` uploads whatever file you point `--file` at to the task —
  confirm the path and that its contents are safe to share.
- `download-attachment` writes to the local filesystem only. Choose an explicit
  safe `--output` path; it will not overwrite existing files unless you pass
  `--overwrite`.
- The token is never printed in output, errors, or `--verbose` logs. Don't echo
  it yourself.
