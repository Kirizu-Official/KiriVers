---
title: "C++ SDK"
description: "kirivers::Client。CheckStatus::Update。hosted vs KIRIVERS_EMBEDDED。nlohmann/json 写死。"
---

# C++

C++17。无 registry。`git clone -b sdk/cpp`。JSON：vendored nlohmann/json（非适配器）。不要 cpp-httplib / RapidJSON。

## 1. 安装

```text
git clone -b sdk/cpp https://github.com/Kirizu-Official/KiriVers.git kirivers-client-cpp
cmake -S . -B build && cmake --build build
```

其它工程：`target_link_libraries(my_app PRIVATE kirivers::client)`。

embedded：`-DKIRIVERS_EMBEDDED=ON` 关掉 curl/openssl/libzip/utf8proc。

## 2. Quick start

```cpp
kirivers::Client client({.base_url = "http://127.0.0.1:8080", .project_ref = "my-app"});
auto st = client.check({
    .current_version = "1.0.0",
    .os = "linux",
    .arch = "x86_64",
    .channel = "stable",
    .device_id = "your-stable-device-id",
});
if (st.status == kirivers::CheckStatus::NoUpdate ||
    st.status == kirivers::CheckStatus::NotModified) {
  return; // HTTP 204 / 304
}
```

`kirivers::Client`。无 Replacer 时 `Updater.run` 只暂存。

## 3. 下载并核对

按 `package_url` 下载并比对 SHA-256。

## 4. Updater

`Updater.run` 无 Replacer 只暂存。

## 5. Client 方法

与 C 相同的原生 JSON 集合。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename。

## 6. 配置与鉴权

`kirivers::Config.base_url`、`project_ref`、令牌、PEM。

## 7. 适配器

hosted：CurlTransport、OpenSslHasher、FilesystemStore、LibzipUnpacker。embedded：至少注入 Transport。可选 Windows `MoveFileExReplacer`。Patcher 仅接口。

## 8. 能力位

按活适配器。无 Patcher 无 `binary_delta`。

## 9. Patcher

注入 apply。magic 不交叉解码。官方 `HDIFF13&` 须自备。

## 10. Replacer

可选 `MoveFileExReplacer`。APK/IPA 须自备。

## 11. 错误

读取信封 code。CheckStatus 区分无更新。

## 12. Transport / 测试

注入 Transport。`ctest`，不依赖 Docker。

## 13. 语言特有

hosted vs `KIRIVERS_EMBEDDED`；nlohmann/json 写死；可选 Windows `MoveFileExReplacer`。
