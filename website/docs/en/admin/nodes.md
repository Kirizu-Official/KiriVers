---
title: "Nodes"
description: "Cluster node presence. Skip on a singleton. node-sync is platform-admin only."
---

# Nodes

Drawer **Nodes** is platform-admin only. Page title **Cluster nodes**. Skip this page on a single process. Project members get 403 <ErrorCode code="FORBIDDEN" />.

1. Open drawer **Nodes**.
2. Read **Status**: **Online** / **Offline** (no heartbeat for more than 30s), **Current**, **Name** (YAML `node.display_name`; empty shows the UUID), **Last seen**.
3. Mode chips: **Singleton** or **Cluster**. Multi-node download requires `storage.driver=s3` **and** `cache.driver=redis`.
4. **Node artifact sync** (`node-sync`) is platform-admin only.

CPU percent is currently always 0; memory is Go HeapAlloc / Sys, not host RSS. Concepts: [Multi-node](/en/guide/features/cluster). YAML: [Cluster](/en/guide/config/cluster).
