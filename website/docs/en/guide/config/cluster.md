---
title: "Cluster YAML"
description: "cluster.download is active only with storage.driver=s3 and cache.driver=redis. Public S3 vs local proxy."
---

# Cluster YAML

| Key | Type | Default | Meaning | When to change |
|-----|------|---------|---------|----------------|
| `cluster.download` | `s3` \| `local` | `s3` | Active only when `storage.driver=s3` **and** `cache.driver=redis` | Public URLs vs node proxy |
| `node.display_name` | string | `""` | Label in the admin node list; empty shows the UUID | Identify machines |

`cluster.download` chooses public object URLs versus node proxy; `node.display_name` is only the console node label. Topology and when to enable clustering: [Feature guides · Multi-node](/en/guide/features/cluster).

<<< @/../../configs/config-example.yaml{54-57}
