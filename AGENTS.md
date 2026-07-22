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
  me.go list_workspaces.go list_projects.go search_tasks.go
  get_task.go list_task_stories.go comment_on_task.go update_task.go
  list_task_attachments.go get_attachment.go download_attachment.go add_attachment.go
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
- **Writes:** `comment-on-task` (`POST /tasks/{gid}/stories`) and `update-task`
  (`PUT /tasks/{gid}`). `update-task` builds its `{"data":{...}}` body from only
  the field flags that were `Changed()`; passing an empty string to `--notes`,
  `--due-on`, or `--assignee` sends JSON `null` to clear the field, `--name` may
  not be empty, and at least one field flag is required (else usage error).
  `--due-on` is validated as `YYYY-MM-DD` client-side; `--assignee` accepts a
  user GID or `me` (no email lookup).
- **Persistent flags** live on the root and populate the package-level `opts`
  (`--human`, `--verbose`, `--timeout`).
- **Attachments:** `list-task-attachments` uses `GET /attachments?parent={task_gid}`;
  `get-attachment` uses `GET /attachments/{gid}`; `download-attachment` fetches
  metadata with `opt_fields=gid,name,download_url`, then streams `download_url`
  via `Client.Download`. Downloads write only to `--output`, refuse overwrite
  unless `--overwrite`, and remove partial output files on failure.
  `add-attachment` uploads a local file via `Client.Upload` (multipart/form-data
  `POST /attachments`): fields `parent`=`--task-gid` and `file` (streamed from
  `--file`), with `--name` overriding the display/file name (defaults to the
  file's base name). Missing flags and a nonexistent/unreadable `--file` are
  usage errors (exit 2).

## Testing

- `go test ./...` — no network/token needed.
- Command tests use `runWithServer` (in `commands_test.go`): spins up `httptest`,
  sets `ASANA_ACCESS_TOKEN=tok`, `ASANA_API_BASE=<server>`, and `ASANA_CONFIG`
  to an absent temp path, runs the root command, returns stdout + error. Assert
  exit semantics with `exitCodeFor(err)`.
- `ASANA_API_BASE` is the test-only base-URL seam (read in `buildClient`); it is
  intentionally undocumented for users.
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
