---
title: "Admins"
description: "Platform admins vs project members. LAST_ADMIN / LAST_OWNER. Project members cannot open /admins /geoip /nodes."
---

# Admins

Drawer **Admin Users** is **platform-admin** only. Project members hitting `/admins`, `/geoip`, or `/nodes` get 403 <ErrorCode code="FORBIDDEN" />.

## Platform admins

Instance-level accounts. Create with [kirivers admin add](/en/guide/config/bootstrap-admin) or this page. You cannot delete the last platform admin (<ErrorCode code="LAST_ADMIN" />). Duplicate username: <ErrorCode code="USERNAME_TAKEN" />. Missing target: <ErrorCode code="ADMIN_NOT_FOUND" />.

1. Open **Admin Users**.
2. Click **New admin**. Fill **Username** and **Password** (at least 8 characters).
3. **Rename / Reset password**: leave password empty to keep the current hash.
4. **Never logged in** is JSON `last_login_at` null. Columns also show **Last login** / **Last login IP** and chips **Platform admin** / **Project account**.
5. Before **Delete admin**, confirm it is not the last admin. Copy: the instance must keep at least one admin.

## Project members

Add members on the project **Settings** card **Project members**. Roles: **Owner** / **Admin**. You cannot remove the last owner (<ErrorCode code="LAST_OWNER" />). Unknown member: <ErrorCode code="MEMBER_NOT_FOUND" />. Project-only accounts do not see Admins / GeoIP / Nodes in the global drawer.

Concepts: [Access tokens](/en/guide/features/tokens).
