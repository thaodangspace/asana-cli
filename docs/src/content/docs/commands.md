---
title: Commands
description: Query Asana resources, inspect attachments, and perform explicit writes.
---

## Global options

- `--human` prints readable summaries instead of JSON.
- `--verbose` logs request method and path to stderr; the token is never logged.
- `--timeout duration` sets the HTTP timeout. The default is 30 seconds.
- `--max-retries N` retries transient GET/DELETE requests after the initial request (default 3).
- `--retry-max-wait duration` caps one retry delay, including `Retry-After` (default 30 seconds).
- `--no-retry` disables retries for deterministic one-shot behavior.

The advanced `api` command does not log request bodies or query values with
`--verbose` and never permits overriding the authorization header.

## Read commands

```sh
asana-cli me [--opt-fields FIELDS]
asana-cli list-workspaces [pagination options] [--opt-fields FIELDS]
asana-cli list-projects --workspace-gid GID [pagination options] [--opt-fields FIELDS]
asana-cli list-project-tasks --project-gid GID [pagination options] [--opt-fields FIELDS]
asana-cli list-tag-tasks --tag-gid GID [pagination options] [--opt-fields FIELDS]
asana-cli search-tasks --workspace-gid GID [search and pagination options]
asana-cli get-task --task-gid GID [--opt-fields FIELDS]
asana-cli list-task-stories --task-gid GID [pagination options]
```

`search-tasks` accepts `--text`, `--assignee`, `--completed`, `--limit`, and
`--opt-fields`. The `--completed` filter is tri-state: it is omitted unless you
explicitly pass `--completed=true` or `--completed=false`. Search may require a
premium Asana workspace.

## Attachment commands

```sh
asana-cli list-task-attachments --task-gid GID [pagination options]
asana-cli get-attachment --attachment-gid GID [--opt-fields FIELDS]
asana-cli download-attachment --attachment-gid GID --output PATH [--overwrite]
asana-cli add-attachment --task-gid GID --file PATH [--name NAME]
```

Downloads refuse to overwrite an existing file unless `--overwrite` is passed.
Failed downloads remove partial output files. Uploads are write operations and
require the appropriate Asana token scope.

## Advanced API command

Use `api` for endpoints that do not yet have a first-class wrapper:

```sh
asana-cli api GET /tasks/123
asana-cli api GET /workspaces/123/tasks/search \
  --query 'projects.any=456' --query 'completed=false'
asana-cli api POST /tasks --data '{"data":{"name":"Ship v2","workspace":"123"}}'
asana-cli api PUT /tasks/123 --data @update.json
asana-cli api DELETE /tasks/123
```

The method must be `GET`, `POST`, `PUT`, `PATCH`, or `DELETE`; the path must be
relative to the Asana API base. Repeatable `--query key=value` flags are safely
encoded, and `--data` accepts validated inline JSON or a JSON file up to 10 MiB.
Absolute/cross-origin paths and custom headers are rejected. Responses unwrap
Asana's `data` field by default; use `--raw-response` to retain the complete
response envelope. Empty successful responses return `data: null`.

## Write commands

```sh
asana-cli comment-on-task --task-gid GID --text "Comment"
asana-cli update-task --task-gid GID --completed
asana-cli update-task --task-gid GID --name "New name" --due-on 2026-07-15
asana-cli update-task --task-gid GID --assignee me --notes "Updated"
```

`update-task` requires at least one changed field. `--notes`, `--due-on`, and
`--assignee` accept an explicit empty string to clear those values; `--name` may
not be empty. Treat `comment-on-task`, `update-task`, and `add-attachment` as
mutating commands and run them only when explicitly requested.

## Pagination

Every list/search command accepts:

- `--limit N` — maximum items (default 20, or 100 for project/tag tasks).
- `--all` — follow pages until `next_page` is absent. This cannot be combined
  with an explicitly supplied `--limit`.
- `--offset TOKEN` — start at an Asana pagination offset.
- `--max-pages N` — positive safety bound; the default is 10. `--all` is
  unlimited unless `--max-pages` is explicitly supplied.

Requests use a page size of 50. JSON responses include `pagination` metadata;
when traversal stops with another page available, `truncated` is true and the
next offset/path is included. Human output says when results are truncated.

## Retries

The client retries `429`, `500`, `502`, `503`, and `504` responses and temporary
network failures for replayable GET/DELETE requests. A `Retry-After` header is
honored in integer-seconds or HTTP-date form, subject to `--retry-max-wait`;
otherwise exponential backoff with jitter is used. Retries are bounded by both
`--max-retries` and `--timeout`, and stop immediately when the context is
canceled. JSON POST/PUT requests and multipart uploads are one-shot to avoid
duplicating mutations. Use `--verbose` to see safe retry method/path, attempt,
and wait details; credentials and query values are never logged.
