---
title: "SDK"
description: "Official SDKs wrap native JSON only. One page per language: install, Quick start, full reference."
---

# SDK

Official SDKs wrap **native JSON** only (check, download, verify, optional delta/pack, optional replace). They do **not** implement Sparkle / electron-updater / WinGet store protocols.

Read install → Quick start until one `check` succeeds. Custom Transport / delta / in-use file replace / embedded networking: the rest of that language page plus [Concepts](./concepts). Store protocols: Electron/Sparkle scenario pages.

Treat documented versions such as `0.1.0` as placeholders for the current published version.

## Install matrix

| Language | Install | Minimum | Source branch |
|----------|---------|---------|---------------|
| Python | `pip install kirivers-client` | Python 3.10+ | `sdk/python` |
| Java | Maven `official.kirizu:kirivers-client` | JDK 17 | `sdk/java` |
| Kotlin | `official.kirizu:kirivers-client-kotlin` (not a Java wrapper) | JDK 17 | `sdk/kotlin` |
| C# | `dotnet add package Kirizu.KiriVers.Client` | .NET 8 | `sdk/csharp` |
| TypeScript | `npm install @kirizu/kirivers-client` | Node 20+ / Electron main; **not browsers** | `sdk/typescript` |
| Rust | crates.io `kirivers-client` | Blocking API, no tokio required | `sdk/rust` |
| Dart | `dart pub add kirivers_client` | Dart 3; Flutter apps yes, **Web no** | `sdk/dart` |
| C | `git clone -b sdk/c https://github.com/Kirizu-Official/KiriVers.git` | C11; CMake hosted or embedded | `sdk/c` |
| C++ | `git clone -b sdk/cpp …` | C++17 | `sdk/cpp` |
| Go | `go get github.com/Kirizu-Official/KiriVers-SDK-Go` (**separate repo**) | Go modules | Publish repo; source `sdk/go-src`; `sdk/go` is a pointer |
| PHP | `composer require kirizu/kirivers-client` (repo `KiriVers-SDK-PHP`) | PHP 8.2+, no Guzzle | `sdk/php-src`; `sdk/php` is a pointer |
| Swift | SPM `https://github.com/Kirizu-Official/KiriVers-SDK-Swift.git` | Apple toolchain; do not add the server repo in Xcode | `sdk/swift-src`; `sdk/swift` is a pointer |

::: warning Packages or dedicated GitHub repositories may be unpublished
Each language page includes a clone fallback. **Do not** `go get` / Packagist / SPM the server repo `main` or pointer branches `sdk/go`, `sdk/php`, `sdk/swift`.
:::

## Shared rules

1. The caller supplies `base_url`, `project_ref`, current version, os/arch, channel, and a stable app-generated `device_id` (the SDK never mints device identity).
2. HTTP 204 / `no_update` means up to date, not an error; 304 is an ETag hit.
3. Without a Replacer, do not describe one-click install.
4. Without a Patcher, do not advertise `binary_delta`.
