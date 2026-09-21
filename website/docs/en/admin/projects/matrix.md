---
title: "Platform matrix"
description: "(os, arch) and package_type. Type locks after a published line. platforms/catalog."
---

# Platform matrix

Nav **Platform Matrix**. Each row is `(os, arch)` plus **Package type** (**Single file** / **Multi file**). OS/Arch comboboxes use catalog slugs (darwin stores as macos). Do not treat display names as identity.

1. Open **Platform Matrix**.
2. Click **Register platform**.
3. Set **OS**, **Arch**, **Package type**, optional **Default delta algo**, **Delta source count**, **HW variant policy**, **Min OS**, **Min API level**, **Fallback arch**.
4. After a **published** line exists for that pair, package type cannot change (<ErrorCode code="PACKAGE_TYPE_IMMUTABLE" />). Copy: “Package type is locked once a version has been published for this platform.”

Type decides incremental path: single file may use binary delta; multi-file uses pack. Concepts: [Platform matrix](/en/guide/features/matrix).
