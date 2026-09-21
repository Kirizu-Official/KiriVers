---
title: "Hardware revisions"
description: "Binding an unregistered rev → HW_REV_UNKNOWN. Not part of the version identity."
---

# Hardware revisions

Nav **Hardware Revisions**. `hw_rev` distinguishes hardware variants of the same firmware and is **not** part of the version identity. Higher **rank** is newer.

1. Open **Hardware Revisions**.
2. Click **Add revision**.
3. Fill **Slug** (normalized `^[a-z0-9_-]{1,64}$`), **rank**, optional **Notes**.

Binding an unregistered rev → <ErrorCode code="HW_REV_UNKNOWN" />. Client sends an incompatible rev → <ErrorCode code="HW_REV_INCOMPATIBLE" />. Hash downloads ignore `X-Hw-Rev`. Concepts: [Hardware revisions](/en/guide/features/hw-revs).
