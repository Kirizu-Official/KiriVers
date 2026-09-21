---
title: "Bootstrap the first admin"
description: "Create the first console login with kirivers admin add on the machine that runs the binary."
---

# Bootstrap the first admin

After the process can start, create the first web account on **the machine that runs the binary**. This is CLI, not self-registration. Finish this page before [Admin → Quick start](/en/admin/quick-start).

`kirivers admin` reads only the system YAML (PostgreSQL DSN), not `admin.yaml`.

## Create

1. Open a terminal in the directory that holds the binary and YAML files (or pass `-config`).
2. Run:

```bash
kirivers admin add alice
```

3. Set the password at the prompt (or `-password`). Success:

```text
created alice (<uuid>)
```

4. Open the admin plane (default `http://127.0.0.1:8081`) and sign in. The first login must enroll TOTP (see [Sign-in and 2FA](/en/admin/security)).

## Other actions

| Command | Effect | Success stdout |
|---------|--------|----------------|
| `kirivers admin list` | List instance admins | Header `USERNAME ID CREATED_AT` |
| `kirivers admin reset-password alice` | Reset password | `password reset for alice` |
| `kirivers admin clear-2fa alice` | Clear TOTP / passkeys / recovery codes | `cleared 2FA for alice` |
| `kirivers admin delete alice` | Delete the account | `deleted alice` |

`clear-2fa` does **not** revoke existing sessions. Deleting the last instance admin fails (<ErrorCode code="LAST_ADMIN" />).
