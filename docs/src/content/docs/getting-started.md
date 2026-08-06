---
title: Getting started
description: Install asana-cli, configure credentials, and make a safe first request.
---

## Requirements

- Go, if installing or building from source
- An Asana personal access token
- Access to the workspace and resources you want to query

## Install

The recommended Homebrew installation is:

```sh
brew tap thaodangspace/tap
brew install asana-cli
```

Or install the latest release with Go:

```sh
go install github.com/thaodangspace/asana-cli/cmd/asana-cli@latest
```

Verify the command:

```sh
asana-cli --help
asana-cli --version
```

## Configure credentials

Set the token in the environment for a session:

```sh
export ASANA_ACCESS_TOKEN="your-asana-personal-access-token"
export ASANA_DEFAULT_WORKSPACE="workspace-gid" # optional
```

Alternatively, create `~/.config/asana-cli.yaml`:

```yaml
access_token: your-asana-personal-access-token
default_workspace: workspace-gid
```

Environment values take precedence over the file. Workspace-scoped commands use
`ASANA_DEFAULT_WORKSPACE` unless you pass `--workspace-gid` explicitly.

## Make a read-only request

Start with the current authenticated user:

```sh
asana-cli me
```

Then list workspaces without modifying Asana:

```sh
asana-cli list-workspaces --limit 10 --human
```

Resolve an assignee, inspect teams and memberships, then create a tag for a
workflow:

```sh
asana-cli find-user --workspace-gid WORKSPACE --email person@example.com
asana-cli list-workspace-teams --workspace-gid WORKSPACE
asana-cli list-workspace-memberships --workspace-gid WORKSPACE
asana-cli create-tag --workspace-gid WORKSPACE --name "Ready" --color light-green
```

Pass the returned user or tag GID to task commands such as `create-task` or
`add-tag-to-task`. `find-user` requires exactly one email match and never picks
one silently when the result is ambiguous.

See [Configuration and security](/configuration-security/) for token handling
and [Commands](/commands/) for the available operations.
