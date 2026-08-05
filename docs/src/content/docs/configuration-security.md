---
title: Configuration and security
description: Manage Asana credentials and understand the CLI's API boundary.
---

## Credential precedence

`asana-cli` resolves credentials in this order:

1. `ASANA_ACCESS_TOKEN` and `ASANA_DEFAULT_WORKSPACE` environment variables
2. `~/.config/asana-cli.yaml` (or the path selected by `ASANA_CONFIG`)

Environment values win when both sources define a value. A default workspace is
optional, but workspace-scoped commands require it or an explicit
`--workspace-gid`.

Restrict the config file to your user:

```sh
chmod 600 ~/.config/asana-cli.yaml
```

Recommended scopes depend on the operation: read scopes for queries, `stories:write`
for comments, `tasks:write` for updates, and attachment read/write scopes for
attachment operations.

## API boundary

All requests use the Asana API at `https://app.asana.com/api/1.0`. The CLI does
not store tokens itself, echo them, or expose a user-facing API-base override.
The `ASANA_API_BASE` environment variable is a test-only seam and should not be
used as production configuration.

Write commands are explicit: commenting, updating tasks, uploading/deleting
attachments, and attaching URLs change remote data. Local attachment uploads
stream with bounded memory. `add-attachment-url` validates HTTPS locally and
passes the URL to Asana without fetching it. Download requests send the PAT
only to the configured Asana origin; external HTTPS downloads and redirects are
unauthenticated, and external non-HTTPS downloads are rejected. The default
test suite uses an in-process HTTP server and does not require a network
connection or real token.

:::danger[Protect credentials]
Never put a real access token in shell history committed to scripts, docs, or
source control. Use environment injection or the private config file instead.
:::
