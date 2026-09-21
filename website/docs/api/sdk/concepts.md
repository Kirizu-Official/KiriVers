---
title: "SDK 概念"
description: "原生 JSON 流程、能力位、适配器、差量 magic、明确不做。"
---

# SDK 概念

跨语言进阶说明。语言细节在各语言页。

## 范围

封装原生客户端 JSON：检查、下载、校验、可选差量/打包、可选替换。

明确不做：管理平面 SDK；Sparkle/WinGet/electron-updater/Play/App Store；OpenAPI 生成客户端；浏览器与 Deno（TypeScript 无 browser export）。

## 默认流程

1. 调用方传入身份参数（含稳定 `device_id`）。SDK 不编身份、不把明文设备编号写入日志。
2. 可选设备报到。
3. POST 检查更新（默认能力只有整包）。
4. 可选更新说明（check 响应不含正文）。
5. 下载 `/packages/{sha256}`（保留 `exp`/`sig`，支持 Range）。
6. SHA-256；若有 `signature` 则按检查载荷验签。
7. 有 Replacer 才安装，否则返回已校验暂存路径。

## 检查与 HTTP

检查更新是 **POST**；GET 同路径 404。204 无更新、304 ETag 命中，都不是失败。错误看 `{ "error": { "code", "message", "details" } }`。

## 能力位

只申报已经接上、真能用的适配器：`full_package` / `file_list` / `patch_package` / `binary_delta`（加 `accepted_delta_algos`）。未注入差量应用器时禁止报 `binary_delta`。`local_sha256` 只出现在差量请求。

## 适配器

HTTP、文件、哈希、zip、验签可注入。JSON **不是**适配器（Java Jackson、Kotlin kotlinx.serialization、Rust serde_json、C cJSON、C++ nlohmann/json 等写死）。不要再叠第二套 HTTP/JSON 库（axios、OkHttp、Guzzle、Gson、RapidJSON 等）。差量 apply 与占用文件/APK 没有默认实现：只留 Patcher / Replacer（Go 可另提供可选纯 Go Patcher）。

## 差量 magic

不得交叉解码：`KVDIFFHP1`、`HDIFF13&`、`BSDIFF40`、VCDIFF `D6 C3 C4`。未知 magic 回退整包。见 [FAQ · 差量](/faq/delta)。

## Docker

SDK 包、安装步骤、调用方应用都不依赖 Docker。契约测试注入假 Transport。Swift `swift test` 需要 Apple 工具链。

## 明确不做

管理平面 SDK；商店 feed 客户端；OpenAPI Generator / hey-api 生成客户端；浏览器 / Deno 更新器；把本服务器仓库的 `main` 当作 Go/PHP/Swift 包根。
