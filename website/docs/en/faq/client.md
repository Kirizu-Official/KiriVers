---
title: "Client sees no update"
description: "Unpublished, os/arch, gray, GET check, channel token is not 403, MIN_OS_NOT_MET, NO_SAFE_TARGET, VERSION_REVOKED."
---

# Client sees no update

| Check | Meaning |
|-------|---------|
| Version status | Must be published with a ready line (`packs_ready_at` stamped) |
| os/arch | Matching version line required |
| Incomplete gray | Non-allowlisted (including anonymous) keep the old version |
| Method | **POST** check only; GET is 404 |
| Channel token | Wrong token is **not** 403; that hidden channel is skipped |
| `MIN_OS_NOT_MET` | OS/API below the line floor |
| `NO_SAFE_TARGET` / `VERSION_REVOKED` | No safe target after revoke/yank |

204 means up to date, not failure. 304 is ETag.
