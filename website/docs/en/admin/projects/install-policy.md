---
title: "Install policy"
description: "Dedicated resource. Channel overlays project on the same platform. Stamps only new Manifests. Reference uses in-channel CompareVersions."
---

# Install policy

Panel and channel dialog titles: **Install policy templates** / **Install policy**. This is `install-policy-rules`, not project PATCH JSON. Empty matrix copy: “Register a platform matrix row before editing install policy.”

1. Open **Settings** → **Install policy templates**, or **Channels** → **Install policy**.
2. Pick **Platform (os/arch)**. Unknown pair → 400 <ErrorCode code="INVALID_REQUEST" />.
3. Project-scope rules use a nil channel UUID; channel rules overlay by path on the same `(os, arch)`.
4. **Add path** with **Path (NFC, forward slashes)**, or tick files from **Files from the latest version on the reference channel**. The reference Manifest is the newest ready version in that channel via `CompareVersions`, **not** `GET versions?latest=true`.
5. **Keep if exists** (`KEEP_IF_EXISTS`) is metadata only; it is not written into the zip. Integrity still uses Manifest SHA-256. Ignore `_keep.json` / `keep_if_exists.txt`. Only **later new** Manifests are stamped.

Illegal path: <ErrorCode code="INVALID_PATH" />. Unknown channel: 404 <ErrorCode code="CHANNEL_NOT_FOUND" />. Concepts: [Install policy](/en/guide/features/install-policy).
