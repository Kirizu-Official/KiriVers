---
title: "Servers and CLI"
description: "Headless processes POST check on start and poll on an operations cadence. The product does not define a fixed interval."
---

# Servers and CLI

## Package shape

Headless binaries (Linux services, Windows services, CLI). Ship as a single file or a directory; see [Desktop](./desktop). Store feeds are usually the wrong protocol.

## Client duties

1. **POST** check when the process starts.
2. Poll afterward on your operations cadence. The product does **not** define a fixed interval. Honor `Retry-After` when present.
3. Without a Replacer: stage the verified file and let systemd / Task Scheduler / the orchestrator replace the running binary.
4. Optional `POST .../telemetry/report`; failures must not block the update.

Related: [Update check](/en/guide/features/check), [Web / Node](./web-node).
