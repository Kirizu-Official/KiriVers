---
title: "Backup and upgrade"
description: "Back up PostgreSQL, object storage or local.root, the three YAML files, and logs. Replace the binary. No migrate CLI."
---

# Backup and upgrade

## Inventory

| Data | Location | Notes |
|------|----------|-------|
| Metadata | PostgreSQL | `pg_dump`; projects, versions, channels, sessions |
| Artifacts | `storage.local.root` or S3/R2 | Packages, deltas, manifests |
| Private objects | `storage.private` | GeoIP MMDB |
| Config | Three YAML files | Secrets stay out of git |
| Logs | Each `log.file.dir` | Optional |

Restore database and objects first, then start the same or a newer binary. Startup runs GORM AutoMigrate. There is **no** `kirivers migrate` command.

## Binary upgrade

1. Back up the table above.
2. Stop the process (systemd / Task Scheduler / `docker stop`).
3. Replace the Release binary or pull a new image. Official artifacts already embed the admin UI.
4. Start; `GET /api/v1/health` must be HTTP 200 with `ready=true` (DB + storage Head `.ready`).

Read that Release’s notes before a major jump. `admin.yaml` `static_dir` is the admin UI tree, not the documentation site.
