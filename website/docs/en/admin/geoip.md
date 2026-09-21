---
title: "GeoIP"
description: "Platform admins upload MaxMind-compatible .mmdb. MaxMind GeoLite2 and Loyalsoldier Country.mmdb. Deleting a roster row is not a privacy wipe."
---

# GeoIP

Drawer title **GeoIP databases**. **Platform admins** only. Project members hitting `/geoip` get 403 <ErrorCode code="FORBIDDEN" />. Unknown database: 404 <ErrorCode code="GEOIP_NOT_FOUND" />.

Uploads must be MaxMind-compatible **`.mmdb`**. The process queries by **rank** ascending: `City` then `Country`, fusing ISO `country_code` / `region_code` and `Names` maps. Files go to **private** keys `geoip/{id}/…`, never the public bucket. MMDB files are not stored in the git tree; upload them in the console.

## Who can open it

1. Sign in as a platform admin.
2. Open the global drawer **GeoIP**.

## Obtain a `.mmdb`

The console accepts only the inner **`.mmdb`**. Do not upload `.tar.gz`, CSV, or `.dat`.

::: code-group

```md [MaxMind GeoLite2]
1. Open https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/
2. Create an account and a license key.
3. From Download Files, get **GeoLite2-Country.mmdb** (country) and/or **GeoLite2-City.mmdb** (country + subdivision for region_code).
4. If you received a `.tar.gz`, extract and upload only the `.mmdb`.
5. Follow the GeoLite [EULA](https://www.maxmind.com/en/geolite2/eula) and keep the DB reasonably current. ASN databases are not required for the console country map.
```

```md [Loyalsoldier]
1. Open https://github.com/Loyalsoldier/geoip/releases
2. Download **Country.mmdb** (MaxMind format).
3. Optional siblings: `Country-without-asn.mmdb`, `Country-asn.mmdb`, `Country-only-cn-private.mmdb` (CN/private-focused, not a full world Country DB).
4. **Do not** upload `geoip.dat` or other `*.dat` (V2Ray/Xray), or Clash/Surge rulesets. Non-`.mmdb` → <ErrorCode code="INVALID_REQUEST" />.
```

:::

## Upload

Page copy: “Upload MaxMind-compatible .mmdb files. Lookup fuses by rank. At most 8 databases, 256 MiB each.” File hint: “Accepts .mmdb only. Do not commit licensed databases to git.”

| Field | UI | Meaning |
|-------|----|---------|
| Name | **Name** | Console label |
| File | **File** | Must be `.mmdb` |
| Rank | **Rank** | Lower rank is queried first (**Move up** / **Move down**) |

1. Click **Upload MMDB**.
2. Fill **Name**, choose a `.mmdb`, set **Rank**.
3. Confirm. Caps: **at most 8** databases, **256 MiB** each. Over cap or wrong type → <ErrorCode code="INVALID_REQUEST" />.

::: warning Private storage
If `storage.driver=s3` and `storage.private` has no bucket, upload fails. GeoIP **must not** land in the public bucket. With `storage.driver=local`, GeoIP shares the disk backend but still uses `geoip/{id}/…`. See [Remote storage](/en/guide/config/s3).
:::

Lookups are in-memory. Reload skips unchanged objects via Head. Roster country/region come from report-time resolution.

## Deleting a database is not a privacy wipe

**Delete GeoIP database** only unloads that MMDB reader. Deleting a **Clients** roster row is also **not** a privacy wipe. Hash-based erase lives on Settings → **Privacy** and returns `telemetry_deleted` / `allowlist_deleted`. See [Clients](/en/admin/projects/clients) and [Telemetry and devices](/en/guide/features/telemetry).
