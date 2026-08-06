# AGENTS.md — asana-cli

Cached repo memory for agents working in this project. Keep this current.

## What this is

`asana-cli` is a standalone Go CLI that exposes Asana operations for consumption
by other agents and scripts. It was originally ported from the
`pi-extensions/asana` TypeScript Pi extension and now also includes attachment
metadata/download support. Output is JSON by default; `--human` gives text
summaries.

## Layout

```
cmd/asana-cli/main.go          # entrypoint: cli.Execute() -> os.Exit
config/                        # credential loading (env + ~/.config/asana-cli.yaml) + workspace resolution
  config.go                    #   Load() (file+env, env wins), LoadFrom(getenv) (env-only), ConfigPath, Config.ResolveWorkspace
asana/                         # HTTP client (ported from src/asana-client.ts plus binary download/upload helpers)
  client.go                    #   Client.Request, Client.Paginate, Client.Download, Client.Upload, HTTPError, EncodePathSegment
cli/                           # Cobra command tree (one file per subcommand)
  root.go                      #   root cmd, persistent flags, exitCodeFor, usageError type
  run.go                       #   buildClient, withTimeout, validateLimit, requireFlag, query helpers, requestData
  output.go                    #   {ok,data} / {ok,error} envelopes, summarizers, humanList
  me.go identity_commands.go tag_commands.go list_workspaces.go list_projects.go search_tasks.go
  get_task.go list_task_stories.go comment_on_task.go update_task.go
  list_task_attachments.go list_attachments.go get_attachment.go download_attachment.go
  add_attachment.go add_attachment_url.go delete_attachment.go
  custom_fields.go               #   definitions, enum options, settings, and typed task values
  api.go
  task_graph.go                  #   subtasks, parents, dependencies, projects, tags, followers
docs/                           # Astro/Starlight static documentation site
```

## Conventions

- **Error typing:** return a `*usageError` (via `usageErrorf` or wrapping) for
  usage/config problems → exit code 2. Any other error → exit code 1. `main`
  renders the error envelope and calls `exitCodeFor`.
- **Output:** commands call `writeSuccess(cmd.OutOrStdout(), data, opts.human, humanText)`.
  Single-resource commands pass the unwrapped object; list commands pass
  `[]json.RawMessage` and build `humanText` via `humanList(...)`.
  `download-attachment` passes a small result struct (`gid`, `name`,
  `output_path`, `bytes_written`).
- **Data unwrapping:** `requestData` strips Asana's top-level `{"data": ...}`;
  `Paginate` returns the accumulated `data` array elements.
- **Pagination:** page size 50, max 10 pages, capped at `--limit` (1..100).
  Constants `pageSize` / `maxPages` in `run.go`.
- **Tri-state flags:** detect explicit set with `cmd.Flags().Changed(name)`
  (see `--completed` in `search_tasks.go`).
- **Writes:** `comment-on-task` (`POST /tasks/{gid}/stories`), task lifecycle
  commands, and `task_graph.go` relationship endpoints are one-shot mutations.
  `update-task` (`PUT /tasks/{gid}`). `update-task` builds its `{"data":{...}}` body from only
  the field flags that were `Changed()`; passing an empty string to `--notes`,
  `--due-on`, or `--assignee` sends JSON `null` to clear the field, `--name` may
  not be empty, and at least one field flag is required (else usage error).
  `--due-on` is validated as `YYYY-MM-DD` client-side; `--assignee` accepts a
  user GID or `me` (no email lookup).
- **Persistent flags** live on the root and populate the package-level `opts`
  (`--human`, `--verbose`, `--timeout`, `--max-retries`, `--retry-max-wait`,
  `--no-retry`).
- **Generic API:** `api METHOD PATH` supports authenticated GET/POST/PUT/PATCH/DELETE
  calls to relative paths, repeatable encoded `--query` values, validated inline
  JSON or `@file` bodies (up to 10 MiB), and `--raw-response`. Absolute paths and
  custom headers are rejected; empty successful responses render `data: null`.
- **Identity and tags:** user, team, team-membership, and workspace-membership
  discovery commands use shared pagination and `--opt-fields`; `find-user`
  forces the email field and refuses truncated, zero, or ambiguous exact matches.
  Tags support get/list/create/update/delete; tag deletion requires explicit
  confirmation and colors are validated client-side.
- **Attachments:** `list-task-attachments` is the backward-compatible task-only
  alias for `list-attachments`, which uses `GET /attachments?parent={parent_gid}`
  for task, project, and project-brief parents. `get-attachment` uses
  `GET /attachments/{gid}`; `download-attachment` fetches metadata with
  `opt_fields=gid,name,download_url`, then streams `download_url` via
  `Client.Download`. Downloads write only to `--output`, refuse overwrite unless
  `--overwrite`, and remove partial output files on failure. `add-attachment`
  accepts `--parent-gid` (with deprecated `--task-gid` alias), streams a local
  file through `Client.Upload` using bounded memory, and supports all parent
  types. `add-attachment-url` validates HTTPS locally and passes URL fields to
  Asana without fetching the URL. `delete-attachment` uses DELETE and requires
  explicit confirmation. Missing flags and a nonexistent/unreadable `--file`
  are usage errors (exit 2). Multipart uploads are one-shot and are never
  retried.
- **Custom fields:** definition updates send only explicitly changed flags;
  enum options support creation, update, disable, and ordering; settings use
  `--parent-type project|portfolio`. Task assignments share a typed parser for
  text, number, enum, multi-enum, date, people, and null values. Typed numbers
  use `json.Number` to preserve precision; unsupported premium or permission
  features return Asana's API error.

## Testing

- `go test ./...` — no network/token needed. Retry tests inject a sleeper and
  clock, so they do not wait on real backoff.
- Command tests use `runWithServer` (in `commands_test.go`): spins up `httptest`,
  sets `ASANA_ACCESS_TOKEN=tok`, `ASANA_API_BASE=<server>`, and `ASANA_CONFIG`
  to an absent temp path, runs the root command, returns stdout + error. Assert
  exit semantics with `exitCodeFor(err)`.
- `ASANA_API_BASE` is the test-only base-URL seam (read in `buildClient`); it is
  intentionally undocumented for users.
- The docs site uses npm from `docs/` and emits static output to `docs/dist/`:
  `make docs-build`. Cloudflare Pages uses root `docs`, build command
  `npm run build`, and output `dist`.
- Tests assert the token never leaks into errors.

## How an agent invokes it

Prefer default JSON; parse the `{ok, data|error}` envelope. Branch on the process
exit code (0/1/2) and on `error.status` for HTTP failures. Pass `--workspace-gid`
explicitly or rely on `ASANA_DEFAULT_WORKSPACE`. For screenshots/files, use
`list-task-attachments`, inspect with `get-attachment`, then save with
`download-attachment --output <path>` instead of raw API calls.

## Provenance / parity

Source of truth for behavior is `~/code/pi-extensions/asana/src/*`. If you change
endpoints, query params, error messages, or pagination, keep them aligned with
that extension (and its tests) unless intentionally diverging.

Known intentional divergences (this CLI has, the extension does not):
- `update-task` (`PUT /tasks/{gid}`) — the extension exposes no task-update tool.
- `add-attachment` (`POST /attachments`, multipart) — the extension exposes no
  attachment-upload tool.
