---
title: "Rust SDK"
description: "crates.io kirivers-client。CheckOutcome。阻塞 reqwest+rustls。不必 tokio。"
---

# Rust

阻塞 API（`reqwest` + `rustls-tls`）。不必 tokio。不要再加 hyper/ureq。

## 1. 安装

```toml
[dependencies]
kirivers-client = "REPLACE_WITH_RELEASE"
```

::: warning 尚未上架 crates.io
clone `-b sdk/rust` 后 path 依赖。
:::

## 2. Quick start

```rust
let client = Client::new(Config::new("http://127.0.0.1:8080", "my-app"));
match client.check(&CheckRequest {
    current_version: "1.0.0".into(),
    os: "linux".into(),
    arch: "x86_64".into(),
    channel: Some("stable".into()),
    device_id: Some("your-stable-device-id".into()),
    ..Default::default()
})? {
    CheckOutcome::NoUpdate | CheckOutcome::NotModified => {}
    CheckOutcome::Update(body) => println!("{}", body.package_url),
}
```

阻塞调用，不必 tokio。传入调用方 `device_id`。

## 3. 下载并核对

`client.download(&body.package_url, None)`，比对 `body.sha256`。

## 4. Updater

编排 check → 下载/差量或 pack → SHA-256 → 可选替换。无 Replacer 只暂存。

## 5. Client 方法

project、report、check、changelog、integrity、diff、pack（PackPollOptions）、packages GET/HEAD、catalogs、announcements、telemetry、media、health。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename。

## 6. 配置与鉴权

`Config::new`，项目/渠道令牌，验签 PEM。

## 7. 适配器

Transport reqwest blocking+rustls。JSON serde_json（非适配器）。Hasher sha2。FileStore std::fs + NFC。ArchiveUnpacker zip crate。SignatureVerifier ed25519-dalek + rsa。Replacer `std::fs::rename`。Patcher **无默认**。

## 8. 能力位

空 capabilities 字段时 check 只发 `full_package`。Updater 按活适配器扩展。

## 9. Patcher

注入后才有 `binary_delta`。magic 不交叉解码。

## 10. Replacer

默认 rename 不能处理占用文件或 APK。缺失 Replacer 不是失败。

## 11. 错误

`kirivers_client::Error` 读 `code`。204/304 为 Outcome 变体。

## 12. Transport / 测试

注入 Transport。`cargo test`，不依赖 Docker。

## 13. 语言特有

阻塞 reqwest+rustls；不必 tokio；不要再加 hyper/ureq。
