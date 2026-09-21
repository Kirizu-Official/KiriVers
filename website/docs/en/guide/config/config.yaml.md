---
title: "config.yaml"
description: "System YAML: PostgreSQL DSN, process log, jobs, dynamic_pack, file_list, changelog window, cache."
---

# config.yaml

File: `config.yaml`. Prefix `KIRIVERS_`. Listen addresses are not in this file.

## PostgreSQL

| Key | Type | Meaning | When to change |
|-----|------|---------|----------------|
| `postgres.dsn` | string | libpq DSN | Always change the password in production; `KIRIVERS_POSTGRES_DSN` is valid |

Startup fatals if the database cannot be opened. A runtime database outage makes business `/api` return 404 while `GET /api/v1/health` stays 200 with `ready=false`.

## Process log `log`

| Key | Type | Typical default | Meaning | When to change |
|-----|------|-----------------|---------|----------------|
| `log.level` | string | example `debug`, Viper default `info` | Invalid levels fall back to info | Production `info`/`warn`; `debug` while diagnosing |
| `log.console.enabled` | bool | true | stderr ConsoleWriter | Off when there is no TTY or you only keep journald |
| `log.file.enabled` | bool | true | JSON rolling file | Off if you only want stderr |
| `log.file.dir` | string | `./logs` | Relative to process CWD | Absolute path in production; include in backups |

The four plane log streams live in the plane YAML files. See [Configuration](./).

## Jobs and ceilings

| Key | Type | Default | Meaning | When to change |
|-----|------|---------|---------|----------------|
| `jobs.workers` | int | `2` | Job workers | Delta / pack / cleanup concurrency |
| `dynamic_pack.max_bytes` | int | `536870912` | Uncompressed hard cap for dynamic multi-file packs | Oversize → HTTP 200 `full_package` |
| `file_list.max_files` | int | `16` | Instance ceiling; values `<1` become 16 | Projects may lower, never exceed |
| `changelog.default_entries` | int | `5` | Count without `from_version`; `<1` → 5 | Shorten the default window; must be ≤ `max_entries` |
| `changelog.max_entries` | int | `50` | Truncation with `from_version`; `<1` → 50 | **`default_entries > max_entries` refuses to listen** |

<<< @/../../configs/config-example.yaml{77-95}

## Other keys

- Cache: [Cache and Redis](./cache)
- Storage: [S3](./s3) and [local disk](./local-storage)
- `cluster.download` / `node.display_name`: [cluster](./cluster)
- `security.*` / `url_signing_secret`: [security](./security)
