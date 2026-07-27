---
title: Output and errors
description: Integrate asana-cli safely into scripts and agent workflows.
---

## JSON output

JSON is the default output. Successful commands emit an envelope:

```json
{
  "ok": true,
  "data": {}
}
```

The `data` value is the unwrapped Asana object or array. Downloads return a
small result object containing the attachment ID, name, output path, and bytes
written.

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
