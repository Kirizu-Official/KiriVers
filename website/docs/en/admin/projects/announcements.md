---
title: "Announcements"
description: "Seven scopes. One language per row. Empty match is 200 []. Media is UUID-public."
---

# Announcements

Nav **Announcements**. Announcements are a project resource, not version changelog, and they are not in check JSON.

1. Click **Create announcement**.
2. Pick **Scope**: **Project-wide**, **Version only**, **OS only**, **Architecture only**, **Version + OS**, **Version + arch**, **Version + matrix pair**. **Forbidden**: OS+Arch without a version (400 <ErrorCode code="INVALID_REQUEST" />). Version scope with a missing version → 404 <ErrorCode code="VERSION_NOT_FOUND" />.
3. One **Language** per row. Fill **Title**, optional **Subtitle**, **Markdown**. Images use media (client UUID GET, no token, no `exp`/`sig`). `${site_url}` expands from Referer.
4. Optional **Starts at (UTC window)** / **Ends at**. **Draft** and not-yet-due **Scheduled** are console-only.
5. **Move up** / **Move down** reorder. **Test client API** previews matching. **Delete announcement** drops the item for clients immediately.

Empty client match is **200** `{ announcements: [] }`, not 204. Explicit `locale` is strict. Concepts: [Announcements](/en/guide/features/announcements). HTTP: [Announcements API](/en/api/client/announcements).
