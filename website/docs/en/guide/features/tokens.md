---
title: "Access tokens"
description: "Project, CI, store listing, channel tokens, and admin sessions. Plaintext appears only on create."
---

# Access tokens

Tokens are scoped per plane. Plaintext appears **only on create**; later GET responses omit it.

## When to use

Public projects can skip a project token. Turn tokens on for private check, hidden channels, CI publish, or store-feed auth.

## Configuration entry

- **Tokens**: project and CI tokens. [Tokens](/en/admin/projects/tokens)
- **Settings → Security**: store token, **Require client token for checks**
- **Channels**: channel token
- Admin session: login cookie/Bearer; TTL in [Security settings](/en/guide/config/security)

## Rules and error codes

| Kind | Scope | Failure |
|------|-------|---------|
| Project token | Client plane when `require_client_token`; `X-Project-Token` or `Authorization: Bearer` | 401 <ErrorCode code="UNAUTHORIZED" /> |
| CI token | Admin-plane CI upload/publish | 403 <ErrorCode code="FORBIDDEN" /> off-scope |
| Store listing token | StoreAuth on feeds | 401; missing listing is plaintext 404 |
| Channel token | `X-Channel-Token` | check is **not** 403; skip that channel |
| Admin session | Cookie/Bearer; idle sliding TTL | 401; project members hitting `/admins` get 403 <ErrorCode code="FORBIDDEN" /> |

## Client duties

Do not log tokens. Lost plaintext cannot be recovered; revoke and create again.

Related: [CI releases](/en/api/ci), [Store listings](./store).
