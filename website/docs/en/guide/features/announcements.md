---
title: "Announcements"
description: "Project notices, not in check. Seven scopes. Separate client GET; empty list is 200."
---

# Announcements

Announcements are a **project resource**: ordered notices for software clients. They are not version changelog and they are not in check JSON.

## When to use

Use for outage copy, mandatory text, or campaigns. Release notes belong in [Changelog](./changelog).

## Configuration entry

Project → **Announcements**. Steps: [Announcements](/en/admin/projects/announcements). No YAML file. Announcement GET has its own `s-maxage`.

## Rules and error codes

Clients match on version / os / arch. **Forbidden**: OS+Arch without a version (400 <ErrorCode code="INVALID_REQUEST" />). Version scope with a missing Version → 404 <ErrorCode code="VERSION_NOT_FOUND" />. Bad `version` query → 400 <ErrorCode code="INVALID_QUERY_PARAM" />.

One language per row. Window: `starts_at` / `ends_at`. Draft and not-yet-due `scheduled` are console-only.

Clients **GET announcements** (separate from check). Explicit `locale` is **strict**; omit locale for leftover. Empty match is **200** `{ "announcements": [] }`, not 204. Media UUID GET needs no token. Honor ETag / 304. There is no product-wide poll interval.

Related: [Languages](./languages), [Announcements API](/en/api/client/announcements).
