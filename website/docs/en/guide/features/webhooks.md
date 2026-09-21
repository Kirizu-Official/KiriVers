---
title: "Webhooks"
description: "Notify on publish, line ready, and revoke. Delivery logs live in Settings."
---

# Webhooks

Webhooks POST to your URL on version **publish**, version-line **ready**, and version **revoke**.

## When to use

Drive CI, chat, or external audit. This is not a client update channel.

## Configuration entry

Project **Settings → Webhook**. Fill **Webhook URL**, open **View delivery logs**. Steps: [Settings](/en/admin/projects/settings). No YAML webhook file.

## Rules and error codes

Delivery is an async job with bounded retries, then a delivery row. Do not paste secrets into examples. Schema: [API reference](/en/api/reference/) `openapi.admin.json` (<a href="/en/api/scalar/admin" target="_blank" rel="noopener">open Scalar</a>) Webhooks tag.

## Client duties

None. Software clients do not consume webhooks.

Related: [Releases](./versions), [Jobs](/en/admin/jobs).
