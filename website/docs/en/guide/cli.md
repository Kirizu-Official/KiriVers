---
title: "CLI"
description: "kirivers server and admin add|delete|list|reset-password|clear-2fa. There is no listen subcommand."
---

# CLI

The executable entry is `kirivers`. With no positional arguments it starts the HTTP servers (same as `kirivers server`).

```text
KiriVers — unified startup entry

Usage:
  kirivers [server] [-config path]                              start the HTTP servers (default)
                                                                client plane ... on client.yaml addr (:8080)
                                                                admin plane ... on admin.yaml addr (:8081)
  kirivers admin add <username> [-password secret]              create an instance admin
  kirivers admin delete <username>                              delete an instance admin
  kirivers admin list                                           list instance admins
  kirivers admin reset-password <username> [-password secret]   reset an admin password
  kirivers admin clear-2fa <username>                           clear TOTP, passkeys and recovery codes (does not revoke sessions)
```

There is **no** `kirivers listen` subcommand. `cmd/listen.go` in the repository is TLS listen logic, not a CLI command.

## Config path

| Source | Priority |
|--------|----------|
| `-config path` | Highest |
| `KIRIVERS_CONFIG` | When `-config` is omitted |
| Default `configs/` then the current directory | When neither is set |

`path` may be a file or a directory. Directory: `config.yaml` + `admin.yaml` + `client.yaml` in that folder. File: that system YAML plus sibling `admin.yaml` and `client.yaml`. Missing files fall back to sibling `*-example.yaml`.

`kirivers admin` loads **only** the system `config.yaml` (database connection). It does not read `admin.yaml`.

`-password` belongs to `admin` only. Passing it to `server` prints usage and exits **2**.

The examples below assume the three YAML files are in the working directory or `-config` is set.

## `kirivers` / `kirivers server`

```bash
kirivers -config ./configs
```

Success: the process stays up; logs show client and admin listen addresses. Failure: both log sinks disabled, PostgreSQL unreachable, or `cache.driver=redis` Ping failure → non-zero exit and no listen.

Common mistake: `-password` on server → stderr contains `-password is only valid with the admin command`, exit 2.

## `kirivers admin add`

```bash
kirivers admin add alice
```

Without `-password`, the process prompts on the terminal. Success:

```text
created alice (<uuid>)
```

## `kirivers admin list`

```bash
kirivers admin list
```

```text
USERNAME  ID                                    CREATED_AT
alice     11111111-1111-1111-1111-111111111111  2026-09-19T04:00:00Z
```

## `kirivers admin reset-password`

```bash
kirivers admin reset-password alice -password 'new-secret'
```

Success: `password reset for alice`

## `kirivers admin clear-2fa`

```bash
kirivers admin clear-2fa alice
```

Success: `cleared 2FA for alice`

This clears TOTP, passkeys, and recovery codes. It does **not** revoke existing admin sessions.

## `kirivers admin delete`

```bash
kirivers admin delete alice
```

Success: `deleted alice`. Deleting the last instance admin fails (`LAST_ADMIN`).

Unknown commands or a missing username exit 2 and print usage.
