---
title: "Local disk storage"
description: "storage.driver=local and the root directory. Backing up that directory backs up packages."
---

# Local disk storage

| Key | Type | Default | Meaning | When to change |
|-----|------|---------|---------|----------------|
| `storage.driver` | string | `local` | `local` or `s3` | Switch to s3 for object storage |
| `storage.local.root` | path | `./data/storage` | Object root | Absolute path in production; include in backups |

With `driver: local`, GeoIP and artifacts share the backend (separate keys). Backing up this directory backs up packages. Do not scale to multiple nodes on local disk alone; see [S3](./s3) and [cluster](./cluster).
