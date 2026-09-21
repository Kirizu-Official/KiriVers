---
title: "Incremental updates"
description: "Start from package_type. Single-file uses POST diff and magic; multi-file uses integrity and same-URL pack. Oversize is 200 full_package."
---

# Incremental updates

Incremental updates reduce bytes. Matrix `package_type` decides the path: single-file may use binary delta; multi-file uses a fileset pack. After a published line exists, type is locked (<ErrorCode code="PACKAGE_TYPE_IMMUTABLE" />). Undeclared capabilities fall back to a full package.

## When to use

Use diff for a single-file installer/firmware when a Patcher is injected. Use integrity + pack for directory trees. Store feeds do **not** binary-delta each dll.

## Configuration entry

- Console: **Platform Matrix** package type and default delta algo; version **Generate delta**. Steps: [Platform matrix](/en/admin/projects/matrix), [Versions](/en/admin/projects/versions).
- YAML: `dynamic_pack.max_bytes`, `file_list.max_files` — [config.yaml](/en/guide/config/config.yaml).

## Rules and error codes

### Single file

Check may advertise `binary_delta` when a Patcher is injected and `accepted_delta_algos` is set. Then `POST /update/diff`. `local_sha256` appears **only** on diff. Admin unknown algo → 400 <ErrorCode code="DELTA_ALGO_UNSUPPORTED" />. Same source and target → <ErrorCode code="DELTA_SAME_VERSION" />.

Do not cross-decode container magics: `KVDIFFHP1`, `HDIFF13&`, `BSDIFF40`, VCDIFF `D6 C3 C4`. Unknown magic → full package. Generation is a Job.

### Multi-file

No per-dll binary delta. `GET` integrity, then `POST /update/pack` and **poll the same URL**. Oversize or over `dynamic_pack.max_bytes` returns **200** `status=full_package` (not 400). No admin `job_id`. Unstamped line → <ErrorCode code="VERSION_NOT_VISIBLE" />.

## Client duties

Advertise only wired capabilities. Do not send `binary_delta` without a Patcher. Official SDKs may omit `KEEP_IF_EXISTS` paths from `needed_paths` when the file already exists.

Related: [FAQ · delta](/en/faq/delta), [diff](/en/api/client/diff), [pack](/en/api/client/pack).
