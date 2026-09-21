---
title: "TypeScript SDK"
description: "@kirizu/kirivers-client。Node 20+ / Electron 主进程。无浏览器、无 Deno、无渲染进程。"
---

# TypeScript

::: warning 运行时
**不能用于网页。** 无 browser export，无 Deno，无 Electron 渲染进程。仅 Node.js 20+ 与 Electron **主进程**。
:::

运行时依赖仅 **yauzl**。不要加 axios / node-fetch。

## 1. 安装

```bash
npm install @kirizu/kirivers-client
```

::: warning 尚未上架 npm
clone `-b sdk/typescript` 后本地安装。
:::

## 2. Quick start

```ts
import { Client } from "@kirizu/kirivers-client";

const client = new Client({
  baseUrl: "http://127.0.0.1:8080",
  projectRef: "my-app",
});
const check = await client.check({
  current_version: "1.0.0",
  os: "windows",
  arch: "x86_64",
  channel: "stable",
  device_id: "your-stable-device-id",
});
if (check.kind === "update") {
  console.log(check.body.package_url);
}
// 204 → 非 update；304 → not modified
```

仅 Node 20+ / Electron 主进程。

## 3. 下载并核对

`client.downloadUrl(check.body.package_url)` + `NodeHasher().sha256`。

## 4. Updater

省略 `targetPath` 只暂存。默认 `fs.rename` 不是占用 exe / APK 安装器。

## 5. Client 方法

`health()`、`project()`、`deviceReport()`、`check()`、`changelog()`、`integrity()`、`diff()`、`pack()` / `packUntilReady()`、download/head、catalog、`announcements()`、`reportTelemetry()`、media。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename。

## 6. 配置与鉴权

`baseUrl`、`projectRef`、`projectToken`、`channelToken`。不要 log 令牌。

## 7. 适配器

Transport：Node 20 `fetch`。JSON：`JSON`。FileStore `node:fs`。Hasher `node:crypto`。SignatureVerifier `node:crypto`。ArchiveUnpacker yauzl。Patcher 仅接口。Replacer `fs.rename`（EXDEV 拷贝）。

## 8. 能力位

Updater 按活着的适配器申报。无 Patcher 则无 `binary_delta`。

## 9. Patcher

注入 `supportedAlgos` + `apply`。magic 不交叉解码。

## 10. Replacer

Windows 占用 exe、APK 须自备。默认 rename 只替换你指定的 `targetPath`。

## 11. 错误

`ApiError` 读取 `error.code`。204/304 不是失败。

## 12. Transport / 测试

注入 Transport。不依赖 Docker。

## 13. 语言特有

Node 20+ / Electron 主进程；无 browser/Deno/渲染进程；运行时仅 yauzl。
