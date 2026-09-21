---
title: "Sign-in and 2FA"
description: "Password → pending → second factor. Passkeys require both webauthn_rp_id and webauthn_origins. clear-2fa does not revoke sessions."
---

# Sign-in and 2FA

The login title is **Sign in to KiriVers**. Flow: password → enroll or second factor (pending TTL: `security.login_pending_ttl_seconds`) → full session. Idle sliding TTL: `security.session_idle_hours`. Admin auth uses an **opaque cached session token** — the random string carried in `Authorization: Bearer`.

Who can open this: anyone who can reach the admin plane (default `http://127.0.0.1:8081`). Create the first account with [Bootstrap the first admin](/en/guide/config/bootstrap-admin).

## First sign-in

1. Enter **Username** and **Password**, then **Sign in**.
2. First visit must **Set up authenticator**: scan or type the secret, enter the **6-digit code**, then **Confirm TOTP**.
3. **Save recovery codes** (shown once). Check **I have saved these recovery codes**, then **Continue**.
4. **Add a passkey (optional)** appears. Use **Register passkey**, or **Skip**. Add passkeys later under **Account security**.

Too many TOTP tries in one 30s window: <ErrorCode code="TOTP_RATE_LIMITED" />.

## Configure passkeys (WebAuthn)

Passkeys need **both** system YAML keys. If `security.webauthn_rp_id` **or** `security.webauthn_origins` is empty, Passkey routes return <ErrorCode code="NOT_READY" /> (HTTP 503). TOTP and recovery codes still work.

The console then shows: “WebAuthn is not configured on the server, so passkeys cannot be added. TOTP and recovery codes still work.”

| Key | Type | Default | Meaning | When to change | Failure |
|-----|------|---------|---------|----------------|---------|
| `security.webauthn_rp_id` | string | `""` | Relying-party ID: hostname **without** scheme or port | To enable passkeys | Empty → <ErrorCode code="NOT_READY" /> |
| `security.webauthn_origins` | string[] | `[]` | Full Origin list (scheme + host + port) | Must match the browser Origin | Empty or mismatch → browser reject or <ErrorCode code="NOT_READY" /> |

Example (local admin plane `http://localhost:8081`):

<<< @/../../configs/config-example.yaml{59-70}

Set `webauthn_rp_id` to `localhost` (or a public hostname such as `updates.example.com`) and `webauthn_origins` to a full Origin such as `http://localhost:8081`. **Restart the process after editing YAML.**

::: warning HTTPS
Non-localhost origins need an HTTPS secure context. `http://localhost:8081` is allowed.
:::

::: code-group

```yaml [localhost]
security:
  webauthn_rp_id: localhost
  webauthn_origins:
    - http://localhost:8081
```

```yaml [public]
security:
  webauthn_rp_id: updates.example.com
  webauthn_origins:
    - https://updates.example.com
```

:::

YAML: [Security settings](/en/guide/config/security).

## Account security (add or delete later)

User menu **Account security** (`/security`) is not a drawer item. Copy states this **does not kick other sessions**.

1. Open **Account security**.
2. Under **Passkeys**, click **Add passkey**, set **Passkey name**, complete the browser prompt.
3. Delete with **Delete passkey**. Sessions stay signed in; TOTP is not removed.
4. **Rotate TOTP** / **Regenerate** recovery codes also leave other sessions signed in.

If WebAuthn is not configured, **Add passkey** is unavailable and the console message quoted above is shown. TOTP and recovery codes still work.

## `clear-2fa`

```bash
kirivers admin clear-2fa alice
```

Clears TOTP, passkeys, and recovery codes for that account. It does **not** revoke existing sessions. The next sign-in must enroll TOTP again. See [Bootstrap the first admin](/en/guide/config/bootstrap-admin).
