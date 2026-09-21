---
title: "Security settings"
description: "Sliding session TTL, login pending TTL, TOTP rate limit, WebAuthn rp_id, url_signing_secret."
---

# Security settings

Keys live under `security` and top-level `url_signing_secret` in `config.yaml`.

| Key | Type | Default | Meaning | When to change |
|-----|------|---------|---------|----------------|
| `security.session_idle_hours` | int | `72` | Full admin session **idle sliding TTL**; admin auth looks up the cached session token (opaque random string) | Shorter idle logout |
| `security.login_pending_ttl_seconds` | int | `600` | Restricted token after password (enroll / second factor) | Shorten the enroll window; expiry requires a new password login |
| `security.totp_max_attempts_per_period` | int | `10` | Attempts per 30s step; excess → <ErrorCode code="TOTP_RATE_LIMITED" /> | Lower to tighten brute-force |
| `security.webauthn_rp_id` | string | `""` | Hostname without scheme/port. **Either** this or `webauthn_origins` empty → Passkey APIs <ErrorCode code="NOT_READY" /> (HTTP 503); TOTP still works | Set when enabling Passkeys |
| `security.webauthn_origins` | string[] | `[]` | Full Origin list (scheme + host + port, e.g. `http://localhost:8081`) | Must match the browser Origin; restart after YAML change |
| `url_signing_secret` | string | `""` | HMAC for short-lived private URLs. Unset → temporary secret at boot (**all signed URLs die on restart**) | Must be explicit in production (`KIRIVERS_URL_SIGNING_SECRET`) |

Passkey steps and Origin examples: [Sign-in and 2FA](/en/admin/security).

<<< @/../../configs/config-example.yaml{59-70}
