---
title: "本机磁盘存储"
description: "storage.driver=local 与 root 目录。备份该目录等于备份安装包。"
---

# 本机磁盘存储

| 参数 | 类型 | 默认 | 说明 | 何时改 |
|------|------|------|------|--------|
| `storage.driver` | string | `local` | `local` 或 `s3` | 上对象存储时改 s3 |
| `storage.local.root` | path | `./data/storage` | 对象根目录 | 生产用绝对路径并纳入备份 |

`driver: local` 时 GeoIP 与产物共用该后端（仍按键隔离）。备份该目录等于备份安装包与相关对象。多节点不要只靠本地盘；见 [S3](./s3) 与 [cluster](./cluster)。
