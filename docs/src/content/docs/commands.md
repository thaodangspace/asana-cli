---
title: Commands
description: Query Asana resources, inspect attachments, and perform explicit writes.
---

## Global options

- `--human` prints readable summaries instead of JSON.
- `--verbose` logs request method and path to stderr; the token is never logged.
- `--timeout duration` sets the HTTP timeout. The default is 30 seconds.

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
