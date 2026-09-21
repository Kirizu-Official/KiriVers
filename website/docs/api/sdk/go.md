---
title: "Go SDK"
description: "模块 github.com/Kirizu-Official/KiriVers-SDK-Go。禁止 go get 本仓。EnableDeltaPatcher 可选。"
---

# Go

模块路径 **`github.com/Kirizu-Official/KiriVers-SDK-Go`**。Go modules 索引仓库根，**不能** `go get` 本服务器仓库 `main` 或跳转分支 `sdk/go`。

## 1. 安装

```text
go get github.com/Kirizu-Official/KiriVers-SDK-Go@latest
```

::: warning 独立仓尚未公开
```text
git clone -b sdk/go-src https://github.com/Kirizu-Official/KiriVers.git KiriVers-SDK-Go
cd KiriVers-SDK-Go
CGO_ENABLED=0 go test ./...
```
不要把本仓当作模块根。
:::

## 2. Quick start

```go
package main

import (
	"fmt"
	"log"

	kirivers "github.com/Kirizu-Official/KiriVers-SDK-Go"
)

func main() {
	c := kirivers.NewClient(kirivers.Config{
		BaseURL:    "http://127.0.0.1:8080",
		ProjectRef: "my-app",
	})
	check, err := c.Check(kirivers.CheckRequest{
		CurrentVersion: "1.0.0",
		OS:             "windows",
		Arch:           "x86_64",
		Channel:        "stable",
		DeviceID:       "your-stable-device-id",
	})
	if err != nil {
		log.Fatal(err)
	}
	if check.NoUpdate || check.NotModified {
		fmt.Println("up to date") // HTTP 204 / 304
		return
	}
	fmt.Println(check.Update.PackageURL)
}
```

传入调用方保存的 `DeviceID`。不要 `go get` 本服务器仓库。

## 3. 下载并核对

`c.Download` + `StdHasher{}.SHA256`。

## 4. Updater

`NewUpdater(client).Update`：报到 → check → 可选 changelog → diff 或 pack → 校验 → 可选替换 → 遥测。无 Replacer 返回暂存路径。

## 5. Client 方法

覆盖客户端 OpenAPI 除商店与遗留路径。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename。

## 6. 配置与鉴权

`Config.BaseURL`、`ProjectRef`、Token、`PublicKeyPEM`。`CGO_ENABLED=0`。

## 7. 适配器

Transport `net/http`。JSON `encoding/json`。Hasher crypto/sha256。FileStore os。ArchiveUnpacker archive/zip。SignatureVerifier ed25519+rsa。NFC `golang.org/x/text`。Replacer `os.Rename`；Windows 占用走 `MoveFileEx`。Patcher 默认关。

## 8. 能力位

默认 Transport+zip+FileStore 会报 `full_package`/`patch_package`/`file_list`。`EnableDeltaPatcher` 或自定义 Patcher 才有 `binary_delta`。

## 9. Patcher

`EnableDeltaPatcher` 可选纯 Go（bsdiff / VCDIFF / `KVDIFFHP1`）。**不** exec `hpatchz`。官方 `HDIFF13&` 仍须调用方注入。

## 10. Replacer

桌面 rename / MoveFileEx。APK/IPA 不是一键安装。

## 11. 错误

Go `error` 携带 `code`。`NoUpdate` 不是 error。

## 12. Transport / 测试

注入 Transport。`CGO_ENABLED=0 go test ./...`。契约测试不启动 Docker。

## 13. 语言特有

独立模块路径；`EnableDeltaPatcher` 可选纯 Go；官方 `HDIFF13&` 仍须自备 Patcher；`CGO_ENABLED=0`。
