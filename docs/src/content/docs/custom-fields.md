---
title: Custom fields
description: Discover and manage Asana custom fields, enum options, and task values.
---

Custom fields are workspace resources. Discover their opaque GIDs before assigning
values to tasks:

```sh
asana-cli list-workspace-custom-fields --workspace-gid WORKSPACE
asana-cli get-custom-field --custom-field-gid FIELD
```

The list command uses the same `--limit`, `--all`, `--offset`, and `--max-pages`
flags as other collection commands. Use `--human` to see each field's name, type,
and GID.

## Definitions

Create, partially update, or delete definitions with the management commands.
Only flags passed to `update-custom-field` are sent to Asana; deletion requires
explicit confirmation.

```sh
asana-cli create-custom-field --workspace-gid WORKSPACE \
  --name Priority --resource-subtype enum
asana-cli update-custom-field --custom-field-gid FIELD \
  --description "Release priority" --precision 0
asana-cli delete-custom-field --custom-field-gid FIELD --yes
```

Supported definition flags include `--name`, `--description`,
`--resource-subtype`, `--type`, `--precision`, `--currency-code`, `--format`,
`--representation-options`, `--enum-options`, `--people-value`,
`--input-restrictions`, `--custom-id-prefix`, `--custom-label`,
`--custom-label-position`, `--is-global-to-workspace`, and `--owned-by-app`.
JSON-valued flags must contain one valid JSON
object, array, scalar, or `null`. Asana may reject fields unavailable to a
workspace, plan, or custom-field subtype; those API errors are returned as
runtime errors without being hidden.

## Enum options

Enum option GIDs can be discovered from a field response (request extra fields
when necessary), then managed directly:

```sh
asana-cli create-enum-option --custom-field-gid FIELD --name "In progress" --color yellow
asana-cli update-enum-option --enum-option-gid OPTION --name "Doing"
asana-cli reorder-enum-option --custom-field-gid FIELD \
  --enum-option-gid OPTION --before OTHER_OPTION
asana-cli disable-enum-option --enum-option-gid OPTION
```

Reordering requires exactly one of `--before` or `--after`. Disabling is used
instead of deleting because enum options may already be assigned to tasks.

## Project and portfolio settings

Asana's settings endpoints use different paths for projects and portfolios, so
pass the parent type explicitly:

```sh
asana-cli list-custom-field-settings --parent-gid PROJECT --parent-type project
asana-cli add-custom-field-setting --parent-gid PROJECT --parent-type project \
  --custom-field-gid FIELD --is-important
asana-cli remove-custom-field-setting --parent-gid PROJECT --parent-type project \
  --custom-field-gid FIELD
asana-cli list-custom-field-settings --parent-gid PORTFOLIO --parent-type portfolio
```

## Task values

`create-task` and `update-task` share one shell-friendly parser. Repeat
`--custom-field` once per field:

```sh
--custom-field FIELD=text:Customer request
--custom-field FIELD=number:12.5
--custom-field FIELD=enum:OPTION_GID
--custom-field FIELD=multi-enum:OPTION_1,OPTION_2
--custom-field FIELD=date:2026-08-15
--custom-field FIELD=people:USER_1,USER_2
--custom-field FIELD=null
```

`number:` values retain their exact decimal representation. Dates are validated
locally as `YYYY-MM-DD`; structured people and multi-enum values reject empty or
duplicate GIDs. An untyped value remains a text value for compatibility, and an
empty value is treated as `null`. The legacy `json:` prefix is also accepted for
advanced JSON values.

Custom fields and search filters can require Asana premium features or suitable
token scopes. The CLI does not assume that every field type is writable; Asana
remains the authority and its permission or plan errors are preserved.
