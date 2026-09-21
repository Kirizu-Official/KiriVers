---
title: "后端分层"
description: "controller 不写业务。service 含 update/ 与 store/。cache fail-closed。"
---

# 后端分层

`controller`（Gin handler，不写业务）→ `service`（含 `update/`、`store/`）→ `repository` → `model`。

横切：`middleware`、`storage`（Local/S3）、`cache`（memory/Redis；`cache.Open` 启动 Ping fail-closed）、`internal/delta`、`internal/platform`。

`pkg/`：`hashutil`、`pathutil`、`response`、`semver`、`signature`、`urlsign`、`webhook`、`grayutil`。check **不得**再走 HMAC gray；`grayutil` 是遗留包。

错误只走 `pkg/response` 信封。管理平面在配置了 `ClientProxyURL` 时反代 `/api/v1/projects/**`。
