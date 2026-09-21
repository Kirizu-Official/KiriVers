---
title: "Changelog"
description: "Body lives only on GET changelog/{channel}/{os}/{arch}. Check has no changelog text."
---

# Changelog

Changelog body lives only on `GET .../changelog/{channel}/{os}/{arch}`. Check JSON has no changelog.

## When to use

Use when the client UI shows a version range. Store protocols still project Version changelog into their document (for example Sparkle `<description>`).

## Configuration entry

Project **Settings → Changelog Defaults** (scope, layout, counts). Click-path: [Settings](/en/admin/projects/settings). Instance ceilings: `changelog.default_entries` / `max_entries` in [config.yaml](/en/guide/config/config.yaml). YAML `default > max` refuses to listen.

## Rules and error codes

| Condition | HTTP | Code |
|-----------|------|------|
| Bad scope/layout or illegal `from_version` | 400 | <ErrorCode code="CHANGELOG_QUERY_INVALID" /> |
| Unknown path channel | 400 | <ErrorCode code="INVALID_QUERY_PARAM" /> |
| Channel token mismatch | 404 | <ErrorCode code="NOT_FOUND" /> |
| Unknown from/to version | 404 | <ErrorCode code="VERSION_NOT_FOUND" /> |
| Project counts above instance ceiling | 400 | <ErrorCode code="INVALID_REQUEST" /> |

No `from_version` returns the newest `default_entries` (product default 5). With from, truncate to `max_entries` (default 50); overflow stays 200. Markdown `${site_url}` expands from RequestOrigin.

## Client duties

GET changelog after check if you need the body. Do not put changelog query params on check.

Related: [Changelog API](/en/api/client/changelog).
