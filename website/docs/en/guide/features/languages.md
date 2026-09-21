---
title: "Languages"
description: "Default locale, changelog and announcement copy. Cap 16. Client GET /languages is separate."
---

# Languages

The project language catalog supplies changelog and announcement rows. Cap 16. Exactly one default locale.

## When to use

Use when changelog or announcements need more than one locale. Compare engine and version identity are language-independent.

## Configuration entry

Project → **Languages**. Steps: [Languages](/en/admin/projects/languages). Default locale at create: [Create a project](/en/admin/projects/create).

## Rules and error codes

Create needs `default_locale`. You cannot delete the last language or the current default (400 <ErrorCode code="INVALID_REQUEST" />). Duplicate: <ErrorCode code="LANGUAGE_TAKEN" />. Missing: <ErrorCode code="LANGUAGE_NOT_FOUND" />. Client `GET /languages` is independent of announcements. Explicit announcement `locale` is strict. Editors must not drop existing keys for other locales.

Related: [Announcements](./announcements), [Changelog](./changelog).
