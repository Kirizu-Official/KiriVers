---
title: "增量更新"
description: "先看 package_type。单文件走 POST diff 与 magic；多文件走 integrity 与同 URL pack，过大 200 full_package。"
---

# 增量更新

增量更新让客户端少下字节。形态由平台矩阵 `package_type` 决定：单文件可走二进制差量；多文件走 fileset pack。该平台已有已发布版本线后形态锁定（<ErrorCode code="PACKAGE_TYPE_IMMUTABLE" />）。未申报对应能力位则只走整包。

## 何时使用

单文件安装器 / 固件且已注入 Patcher 时用 diff。多文件目录用 integrity + pack。商店 feed **不**对每个 dll 做二进制差量。

## 配置入口

- 控制台：**平台矩阵** 的产物形态与默认差量算法；版本 **生成差量**。步骤：[管理 · 平台矩阵](/admin/projects/matrix)、[版本](/admin/projects/versions)。
- YAML：`dynamic_pack.max_bytes`、`file_list.max_files`，见 [服务端配置](/guide/config/config.yaml)。

## 规则与错误码

### 单文件

check 可报 `binary_delta`（已注入 Patcher 并申报 `accepted_delta_algos`）。然后 `POST /update/diff`。`local_sha256` **只**出现在差量请求。管理端未知算法 400 <ErrorCode code="DELTA_ALGO_UNSUPPORTED" />。源与目标相同 <ErrorCode code="DELTA_SAME_VERSION" />。

容器 magic 不得交叉解码：`KVDIFFHP1`、`HDIFF13&`、`BSDIFF40`、VCDIFF `D6 C3 C4`。未知 magic 回退整包。后台生成差量是 Job。

### 多文件

不对每个 dll/so 做二进制差量。客户端 `GET` integrity 对照本地，再 `POST /update/pack`，**同一 URL** 轮询。过大或超过 `dynamic_pack.max_bytes` 返回 **200** `status=full_package`（不是 400）。响应**无**管理 `job_id`。线未预热 <ErrorCode code="VERSION_NOT_VISIBLE" />。

## 客户端职责

只申报已经接上的能力。未注入 Patcher 时禁止 `binary_delta`。官方 SDK 可在文件已存在时从 `needed_paths` 省略 `KEEP_IF_EXISTS` 路径。

相关：[FAQ · 差量](/faq/delta)、[API · diff](/api/client/diff)、[API · pack](/api/client/pack)。
