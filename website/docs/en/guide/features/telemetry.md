---
title: "Telemetry and devices"
description: "Optional report; 202 failures must not block updates. Deleting a roster row is not a privacy wipe. Gray needs roster devices."
---

# Telemetry and devices

Telemetry is optional install-outcome reporting. The operational roster comes from check / report and feeds gray plus overview stats.

## When to use

Enable when you need country charts, install outcomes, or gray allowlists. `device_id_policy=none` cannot admit by device.

## Configuration entry

- **Clients** roster: [Clients](/en/admin/projects/clients)
- **Settings → Privacy**: policy, retention, delete-by-hash
- [GeoIP](/en/admin/geoip) for the country chart

No YAML telemetry switch.

## Rules and error codes

Optional `POST .../telemetry/report` → **202**. Failures must not block download/install. With `device_id_policy=none`, sending a device id may be 400 <ErrorCode code="INVALID_REQUEST" />. Missing roster row → 404 <ErrorCode code="CLIENT_NOT_FOUND" />.

Deleting a **Clients** row is **not** a privacy wipe. Hash erase on Settings → **Privacy** returns `telemetry_deleted` / `allowlist_deleted`. Leftover `POST .../clients/login` is 404.

## Client duties

Ignore report failures. The app generates device identity. Geo is resolved at report time, not a live DB lookup.

Related: [Telemetry API](/en/api/client/telemetry), [Gray rollout](./gray).
