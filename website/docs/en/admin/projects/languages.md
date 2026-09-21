---
title: "Languages"
description: "Envelope { languages: [] }. Create needs a default locale. Cap 16. Client GET /languages is separate."
---

# Languages

Nav **Languages**. Admin envelope `{ languages: [] }`. Create needs a project default. Cap 16. You cannot delete the last language or the current **Default** (400 <ErrorCode code="INVALID_REQUEST" />). Duplicate code: <ErrorCode code="LANGUAGE_TAKEN" />. Missing: <ErrorCode code="LANGUAGE_NOT_FOUND" />.

1. Open **Languages**.
2. Click **Add language**.
3. Fill **Language code** (2–32 chars, case preserved, e.g. `zh-CN`), optional **Display name**, **Sort order**. Tick **Default** when this locale should be the fallback.
4. Changelog / announcement editors keep other locales when you switch; save must not drop existing keys for unselected languages.

Client `GET /languages` is independent of announcements. Concepts: [Languages](/en/guide/features/languages).
