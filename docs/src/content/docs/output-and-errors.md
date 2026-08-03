---
title: Output and errors
description: Integrate asana-cli safely into scripts and agent workflows.
---

## JSON output

JSON is the default output. Successful commands emit an envelope:

```json
{
  "ok": true,
  "data": [],
  "pagination": {
    "pages_fetched": 2,
    "truncated": true,
    "next_offset": "offset-token",
    "next_path": "/workspaces/123/projects?offset=offset-token"
  }
}
```

List and search commands include pagination metadata. `--all` follows every
page; `--offset` resumes a collection; and `--max-pages` can mark a bounded
result as truncated. Single-resource commands omit `pagination`. `data` remains
the unwrapped Asana object or array. Create, update, and duplicate commands
return the affected task; delete-task returns `data: null` even when Asana
responds with an empty `204 No Content` body.

Downloads return a small result object containing the attachment ID, name,
output path, and bytes written. When `truncated` is true, use the returned
`next_offset` or `next_path` to resume when present.

Errors are written to stderr as an envelope:

```json
{
  "ok": false,
  "error": {
    "message": "Asana resource not found.",
    "status": 404,
    "method": "GET",
    "path": "/tasks/999"
  }
}
```

HTTP `status`, `method`, and `path` fields are included for HTTP failures. Use
`--human` when output is intended for a person rather than a parser.

## Exit codes

| Code | Meaning |
| ---: | --- |
| `0` | Success |
| `1` | Runtime error: HTTP failure, network failure, or timeout |
| `2` | Usage/configuration error: missing token, required flag, bad limit, or missing workspace |

Check both the process exit code and the JSON envelope. Diagnostics from
`--verbose` go to stderr, keeping stdout parseable.

## Failure handling

The client never includes the access token in output, errors, or verbose logs.
For a rate limit, authentication failure, authorization failure, or timeout,
fix the reported condition before retrying. Writes are not automatically retried
by the CLI.
