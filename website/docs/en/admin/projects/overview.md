---
title: "Overview"
description: "List and detail stats including storage_bytes, without device hashes. Series from stats/series."
---

# Overview

Nav **Overview**. Project cards and the detail payload include aggregated `stats`: latest version, version count, **7-day active**, `storage_bytes`. The payload has **no** device hashes; list/get already include stats.

1. **Projects** opens the list; search by name or slug.
2. Click **Open project**.
3. On **Overview**, read the **Country** pie (labels follow the current UI language from fused GeoIP names; subdivision ISO is not a country code) and series from `GET .../stats/series`.
4. Jump to **Settings**, **Channels**, **Platform Matrix**, **Versions**, **Announcements**.

Concepts: [Telemetry and devices](/en/guide/features/telemetry). Country chart needs [GeoIP](/en/admin/geoip).
