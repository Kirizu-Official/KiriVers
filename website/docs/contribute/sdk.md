---
title: "SDK 源码分支"
description: "语言在 sdk/lang 干净分支。Go/PHP/Swift 源码在 *-src 加独立仓。默认分支不放语言包。"
---

# SDK 源码分支

每种语言一条孤儿分支 `sdk/<lang>`。Go/PHP/Swift 完整源码在 `sdk/go-src`、`sdk/php-src`、`sdk/swift-src`；`sdk/go` 等仅为跳转 README。不要把语言包目录画进默认分支树。

发布坐标、独立仓步骤、以及 GitHub Actions 自动发版（Conventional Commits、`sdk-<lang>-v*` tag、密钥名）见仓库 `docs/sdk-publish.md`。使用者安装说明在 [API → SDK](/api/sdk/)。不要把维护者 `twine` / `cargo publish` 或 CI 密钥写进使用者页。

## 发版只由「合并进该分支的 PR」触发

- 每条带流水线的 `sdk/<lang>` 分支只在**指向该分支的 PR 被合并**时发版（`pull_request: types: [closed]` + `merged == true`）。直推分支、以及关闭但未合并的 PR 都不发版。
- 合入 `main` 与 SDK 无关；服务端发版是维护者手动的 `workflow_dispatch`，见 [发版与 Docker](/contribute/release)。
- `sdk/c`、`sdk/cpp` 有自己的流水线，但**不进任何 registry**：发版时在 GitHub Release 附一个源码归档 zip（`kirivers-<lang>-<semver>.zip`）。
- `sdk/go`、`sdk/php`、`sdk/swift` 三条跳转分支**没有**工作流；`sdk/*-src` 的流水线在本仓也只跑测试（仓库身份不是发布仓就 `skip`），真正 tag / 发包发生在拷出去的独立仓，见 `docs/sdk-publish.md`。
- 更新日志同样写回各分支的 `CHANGELOG.md`（缺失即创建），不要手改。
