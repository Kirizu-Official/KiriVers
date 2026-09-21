---
title: "Install policy"
description: "Dedicated resource. Stamps only new Manifests. Channel overlays project on the same platform."
---

# Install policy

Install policy is a dedicated resource, not project PATCH JSON. It marks multi-file paths as **Keep if exists** (`KEEP_IF_EXISTS`).

## When to use

Use with multi-file `package_type` when local user files should be kept. Single-file lines have no Manifest path policy.

## Configuration entry

**Settings → Install policy templates** or channel **Install policy**. Steps: [Install policy](/en/admin/projects/install-policy).

## Rules and error codes

On the same `(os, arch)`, channel rules overlay project rules by path. Only **later new** Manifests are stamped. Ignore `_keep.json` / `keep_if_exists.txt`. Reference Manifest uses in-channel `CompareVersions`, not `versions?latest=true`. Unknown matrix pair → 400 <ErrorCode code="INVALID_REQUEST" />. Illegal path → <ErrorCode code="INVALID_PATH" />.

`KEEP_IF_EXISTS` is metadata; integrity still uses Manifest SHA. Official SDKs may omit that path from `needed_paths` when the file already exists.

Related: [Incremental updates](./incremental), [Platform matrix](./matrix).
