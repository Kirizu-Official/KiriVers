---
title: "C SDK"
description: "git clone -b sdk/c。hosted curl 与 KIRIVERS_EMBEDDED 函数指针两套都要写全。"
---

# C

无 registry。C11。CMake。JSON 为 vendored cJSON（非适配器）。不要加 json-c 或第二套 HTTP。

## 1. 安装

```text
git clone -b sdk/c https://github.com/Kirizu-Official/KiriVers.git kirivers-sdk-c
```

hosted：`cmake --preset hosted`。embedded：`KIRIVERS_EMBEDDED=ON`，只留 cJSON + 函数指针。

## 2. Quick start

hosted 先初始化 curl Transport，再构造 Client：

```c
kirivers_transport_curl_init();
KiriversClientConfig cfg = {
  .base_url = "http://127.0.0.1:8080",
  .project_ref = "my-app",
};
KiriversClient *c = kirivers_client_new(&cfg);
KiriversCheckRequest req = {
  .current_version = "1.0.0",
  .os = "linux",
  .arch = "x86_64",
  .channel = "stable",
  .device_id = "your-stable-device-id",
};
KiriversCheckResult result;
int rc = kirivers_client_check(c, &req, &result);
/* KIRIVERS_NO_UPDATE → HTTP 204 */
```

embedded 构建注入函数指针 Transport，不要链接 curl。

## 3. 下载并核对

按 `package_url` GET，比对 SHA-256。保留 `exp`/`sig`，支持 Range。

## 4. Updater

`kirivers_update` 下载并校验后暂存。无默认 Replacer。

## 5. Client 方法

check、download、integrity、diff、pack 轮询、catalogs、announcements、telemetry、health。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename。

## 6. 配置与鉴权

`KiriversClientConfig.base_url`、`project_ref`、项目/渠道令牌。

## 7. 适配器

| 适配器 | hosted | embedded |
|--------|--------|----------|
| Transport | libcurl | **必须注入** |
| FileStore | POSIX/stdio | 注入 |
| Hasher | OpenSSL | 注入 |
| ArchiveUnpacker | libzip | 注入 |
| SignatureVerifier | OpenSSL | 注入 |
| Patcher / Replacer | 无 | 无 |

## 8. 能力位

默认仅 `full_package`。`kirivers_update` 按活 vtable 扩展。无 Patcher 不要报 `binary_delta`。

## 9. Patcher

vtable `supported_algos` + `apply`。magic 分流，未知回退整包。

## 10. Replacer

无默认。固件用 flash 回调。不是一键安装。

## 11. 错误

`KiriversError` 读 code。`KIRIVERS_NO_UPDATE` 不是失败。

## 12. Transport / 测试

embedded 注入 HTTP 函数指针。测试不依赖 Docker。

## 13. 语言特有

hosted：curl/openssl/libzip/utf8proc。embedded：只留 cJSON + 函数指针。无默认 Replacer。
