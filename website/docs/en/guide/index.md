---
title: "Introduction"
description: "KiriVers is a self-hosted software update service: you publish artifacts; apps check and download updates."
---

# Introduction

KiriVers is a **self-hosted software update service**. You publish installers or firmware; applications query this service and download updates. It is not git, and it does not replace App Store, Google Play, or Microsoft Store review.

## Runtime shape

One process exposes two planes:

| Plane | Default address | Role |
|-------|-----------------|------|
| Client plane | `:8080` | Update check, downloads, store feeds, telemetry, announcements |
| Admin plane | `:8081` | Admin console SPA and CI agent |

Official Release binaries embed the admin UI. Operators do not deploy frontend static files separately. PostgreSQL is required; Redis is optional (see [Install](/en/guide/install/) and [Cache](/en/guide/config/cache)).

The CLI is `kirivers` / `kirivers server`, plus local `kirivers admin add|delete|list|reset-password|clear-2fa`. There is no release subcommand and no `kirivers listen`.

## Two integration paths

1. **Native JSON**: the application calls the client plane (`POST .../update/check`). This fits first-party desktop, server, and firmware clients. Official [SDKs](/en/api/sdk/) wrap this path only.
2. **Store listings**: create a listing in the console and point Electron / Sparkle / Tauri (and similar) at the listing URL. Native SDKs **do not** consume feeds.

## Out of scope

- App Store / Play / Microsoft Store publishing.
- Acting as a Linux distro repository (APT / RPM / Flatpak).
- Hosting this documentation site on the admin plane `GET /`.

## Reading order

1. [Choose an install path](/en/guide/install/) → YAML → [create the first admin](/en/guide/config/bootstrap-admin)
2. After login, follow [Admin → Quick start](/en/admin/quick-start) to publish the first version
3. Use [Feature guides](/en/guide/features/) for channels, gray rollout, incremental updates, announcements
4. Then pick a [scenario](/en/guide/scenarios/) for the application type
