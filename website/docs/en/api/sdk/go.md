---
title: "Go SDK"
description: "Module github.com/Kirizu-Official/KiriVers-SDK-Go. Do not go get the server repo. Optional EnableDeltaPatcher."
---

# Go

Module **`github.com/Kirizu-Official/KiriVers-SDK-Go`**. Go modules index a repository root. Do **not** `go get` this server repository’s `main` or pointer branch `sdk/go`.

## 1. Install

```text
go get github.com/Kirizu-Official/KiriVers-SDK-Go@latest
```

::: warning Dedicated GitHub repository not public yet
```text
git clone -b sdk/go-src https://github.com/Kirizu-Official/KiriVers.git KiriVers-SDK-Go
cd KiriVers-SDK-Go
CGO_ENABLED=0 go test ./...
```
Do not treat the server repo as the module root.
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

Pass a caller-owned `DeviceID`. Do not `go get` the server repository.

## 3. Download and verify

`c.Download` + `StdHasher{}.SHA256`.

## 4. Updater

`NewUpdater(client).Update`: report → check → optional changelog → diff or pack → verify → optional replace → telemetry. No Replacer returns the staging path.

## 5. Client methods

Client OpenAPI except store and leftover routes. Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename.

## 6. Config and auth

`Config.BaseURL`, `ProjectRef`, tokens, `PublicKeyPEM`. `CGO_ENABLED=0`.

## 7. Adapters

Transport `net/http`. JSON `encoding/json`. Hasher crypto/sha256. FileStore os. zip. ed25519+rsa. NFC `golang.org/x/text`. Replacer `os.Rename`; Windows busy files `MoveFileEx`. Patcher off by default.

## 8. Capabilities

Default Transport+zip+FileStore send `full_package`/`patch_package`/`file_list`. `binary_delta` requires `EnableDeltaPatcher` or a custom Patcher.

## 9. Patcher

`EnableDeltaPatcher` is optional pure Go (bsdiff / VCDIFF / `KVDIFFHP1`). It does **not** exec `hpatchz`. Official `HDIFF13&` still needs a caller Patcher.

## 10. Replacer

Desktop rename / MoveFileEx. APK/IPA are not one-click.

## 11. Errors

Go `error` carries `code`. `NoUpdate` is not an error.

## 12. Transport / tests

Inject Transport. `CGO_ENABLED=0 go test ./...`. Contract tests do not start Docker.

## 13. Language notes

Separate module path; optional pure-Go `EnableDeltaPatcher`; official `HDIFF13&` still injected; `CGO_ENABLED=0`.
