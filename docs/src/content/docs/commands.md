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
asana-cli list-workspaces [--limit N] [--opt-fields FIELDS]
asana-cli list-projects --workspace-gid GID [--limit N] [--opt-fields FIELDS]
asana-cli list-project-tasks --project-gid GID [--limit N] [--opt-fields FIELDS]
asana-cli list-tag-tasks --tag-gid GID [--limit N] [--opt-fields FIELDS]
asana-cli search-tasks --workspace-gid GID [options]
asana-cli get-task --task-gid GID [--opt-fields FIELDS]
asana-cli list-task-stories --task-gid GID [--limit N]
```

`search-tasks` accepts `--text`, `--assignee`, `--completed`, `--limit`, and
`--opt-fields`. The `--completed` filter is tri-state: it is omitted unless you
explicitly pass `--completed=true` or `--completed=false`. Search may require a
premium Asana workspace.

## Attachment commands

```sh
asana-cli list-task-attachments --task-gid GID [--limit N]
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

List and search commands paginate internally with page size 50 and a maximum
of ten pages. Most commands accept a `--limit` from 1 through 100 (default 20);
`list-project-tasks` and `list-tag-tasks` default to 100 and allow up to 500.
