---
title: "Hardware revisions"
description: "rank, artifact bind, HW_REV_UNKNOWN / HW_REV_INCOMPATIBLE. Not part of the version identity."
---

# Hardware revisions

`hw_rev` distinguishes hardware variants on the same Version Line. It is **not** part of the version identity. Rank defines the compatible range.

## When to use

Use when one firmware image has board revisions. Do not encode hw in SemVer. See [MCU](/en/guide/scenarios/mcu).

## Configuration entry

Project → **Hardware Revisions**; bind on artifact upload. Steps: [Hardware revisions](/en/admin/projects/hw-revs).

## Rules and error codes

Unregistered bind → <ErrorCode code="HW_REV_UNKNOWN" />. Incompatible client rev → <ErrorCode code="HW_REV_INCOMPATIBLE" />. Hash downloads ignore `X-Hw-Rev`.

## Client duties

check may send `hw_rev`. Store feeds project the default hw variant only.

Related: [Platform matrix](./matrix).
