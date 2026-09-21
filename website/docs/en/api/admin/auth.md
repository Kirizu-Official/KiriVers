---
title: "Admin auth"
description: "Password login, pending second factor, session. Passkeys need rp_id and origins."
---

# Admin auth

Admin plane default `:8081`.

1. `POST /api/v1/admin/auth/login` with username and password.
2. If enroll or 2FA is required, use the pending token on TOTP / recovery / WebAuthn routes.
3. After a full session, send Cookie or Bearer.
4. `POST /api/v1/admin/auth/logout` ends the **current** session.

Empty `webauthn_rp_id` **or** empty `webauthn_origins` → Passkey routes `NOT_READY`. TOTP flood → `TOTP_RATE_LIMITED`. CLI `clear-2fa` does not revoke existing sessions. Paths: [API reference](/en/api/reference/) (`openapi.admin.json`; <a href="/en/api/scalar/admin" target="_blank" rel="noopener">open Scalar</a>) Auth tag.
