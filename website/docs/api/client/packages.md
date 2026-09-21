---
title: "下载"
description: "/packages/{ref}，ref 为内容 SHA-256。无 /artifacts/{id}/{filename}。支持 Range 与 exp/sig。"
---

# 下载

`GET|HEAD /api/v1/projects/{project_ref}/packages/{ref}`

`ref` 为内容 SHA-256（可带 `.{ext}` / `.blockmap`）。查找只按哈希。**没有** `/artifacts/{id}/{filename}`。保留 `exp`/`sig`。支持 `Range`。集群 `cluster.download=s3` 时 check 里的 `package_url` 可能是公共对象 URL。

完整字段：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
