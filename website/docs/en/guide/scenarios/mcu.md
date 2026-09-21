---
title: "MCU / firmware"
description: "Integer versions are common. Hardware variants use hw_rev, not the version string. Clients still POST check."
---

# MCU / firmware

## Package shape

Firmware images ship as a **single file**. Compare engine is often **integer build** (chosen before first publish; afterward <ErrorCode code="COMPARE_ENGINE_IMMUTABLE" />).

Hardware revisions of the same image use [hardware revisions](/en/guide/features/hw-revs). Do not encode them in the version identity. Unknown: <ErrorCode code="HW_REV_UNKNOWN" />. Out of range: <ErrorCode code="HW_REV_INCOMPATIBLE" />.

MCU has no store feed: the updater is a homegrown client on native JSON.

## Client duties

1. **POST** check on boot or on your operations cadence (the product does not define a poll interval; honor `Retry-After`).
2. Send `hw_rev`, integer `current_version`, and os/arch.
3. Write the SHA-256 object to flash; official SDKs have no default Replacer or bus driver.

Console: [Hardware revisions](/en/admin/projects/hw-revs), [Release](/en/admin/projects/release).
