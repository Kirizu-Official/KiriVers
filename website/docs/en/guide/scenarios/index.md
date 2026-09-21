---
title: "Scenarios"
description: "Split by artifact shape: single file vs directory, then native JSON or a store feed."
---

# Scenarios

Pick the package shape first, then a protocol.

- **Native JSON**: the app **POST**s [update check](/en/guide/features/check). Official SDKs wrap this path only.
- **Store feed**: when the app already ships Electron / Sparkle / Tauri (or similar), create a store listing and paste the URL into the updater. Official SDKs do **not** read store feeds.

| Shape | Page |
|-------|------|
| Single exe / dmg / AppImage | [Desktop](./desktop) or a store page |
| Multi-file directory | [Desktop](./desktop) multi-file; no per-dll binary delta |
| Electron | [Electron](./electron) (electron-updater generic) |
| Tauri | [Tauri](./tauri) |
| Sparkle / WinSparkle | [macOS / Sparkle](./sparkle) |
| Squirrel / ClickOnce / WinGet / MSIX | [Other desktop updaters](./desktop-feeds) |
| Android sideload / F-Droid | [Mobile](./mobile) |
| Firmware | [MCU](./mcu) |
| Headless services | [Servers and CLI](./server-cli) |
| The website itself | [Web / Node](./web-node) |

Related: [Store listings](/en/guide/features/store).
