---
title: "Platform matrix"
description: "(os,arch), package_type, min OS. Decides incremental path. Type locks after publish."
---

# Platform matrix

The matrix registers each `(os, arch)` **package type**, default delta algo, and hardware policy.

## When to use

Register at least one row before the first package. Type decides incremental path: single-file → diff; multi-file → pack. Scenario pages split by shape; see [Scenarios](/en/guide/scenarios/).

## Configuration entry

Project → **Platform Matrix**. Steps: [Platform matrix](/en/admin/projects/matrix). OS/Arch come from `platforms/catalog`. No YAML table.

## Rules and error codes

After a published line exists, type is locked (<ErrorCode code="PACKAGE_TYPE_IMMUTABLE" />). Min OS/API live on the **version line** (`min_os` / `min_api_level`), not dropped matrix columns. Current line below floor → <ErrorCode code="MIN_OS_NOT_MET" />. Fallback arch is registered on the row.

## Client duties

check `os` / `arch` must canonicalize onto a registered row. Unknown pairs match no artifacts.

Related: [Incremental updates](./incremental), [Hardware revisions](./hw-revs).
