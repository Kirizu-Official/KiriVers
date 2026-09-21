---
title: "CDN 与下载"
description: "public_base_url。check 不能上公共 CDN。当前拒绝 storage_visibility=private。"
---

# CDN 与下载

把 `storage.s3.public_base_url` 指到 CDN。对象键 `{slug}/{sha256}`。

## 何时使用

安装包很大或需要边缘缓存时使用。本机磁盘足够时不必上 CDN。

## 配置入口

YAML `storage.s3.public_base_url`：[远程存储](/guide/config/s3)。`url_signing_secret` 生产必须固定：[安全](/guide/config/security)。控制台 **存储可见性** 锁定 public。

## 规则

<Badge type="warning" text="check 非公共缓存" />

可以边缘缓存：公开产物 GET、商店 feed（**完整灰度**且非 private/local-proxy）、公告 GET、media（immutable）。

不可以当公共缓存：check（含未完成灰度）、签名 URL、local-proxy 下载、带渠道 Token 的 check。

当前管理 API **拒绝**把项目 `storage_visibility` 设为 `private`（400 <ErrorCode code="INVALID_REQUEST" />）。签名 URL 仍用于 local-proxy 等路径。Range 请求与 CDN 的 Range 支持要对齐。未知包哈希 404 <ErrorCode code="NOT_FOUND" />。私有签名失败 403 <ErrorCode code="FORBIDDEN" />。

## 客户端职责

按 `package_url` 下载并核对 SHA-256。保留查询串 `exp`/`sig`。支持 Range。

相关：[检查更新](./check)、[多节点](./cluster)。
