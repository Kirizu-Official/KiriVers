<!--
标题请用 Conventional Commits：`<类型>(<可选 scope>): <一句话说明>`，例如 `fix(client): 修正空响应`。
类型只能是 feat | fix | perf | revert | docs | refactor | test | style | chore | ci | build；scope 用小写。
破坏性变更写成 `类型!:` 或在下面的「破坏性变更」里说明。

进 main 的 PR 必须绑定 Issue（`sdk/*` 分支的 PR 与直推 main 不受此限）：
在右侧栏 Development → Link an issue 关联，或在下面写 `Fixes #编号`；确实不需要时给本 PR 加 `skip-issue-check` 标签。
未通过检查的外部贡献者 PR 会被 pr-guard 自动关闭并留言说明；改好后点 Reopen pull request 即可，守卫会自动复检。
-->

## 关联 Issue / Related Issue

Fixes #

## 这个 PR 做了什么 / What this PR does

<!-- 为什么需要这个改动，而不只是改了什么 -->
<!-- Why this change is needed, rather than just what is changed -->

## 破坏性变更 / Breaking Changes

<!-- 没有就留空。有请写清旧行为、新行为与迁移方式；标题或提交需用 `类型!:` -->
<!-- Empty if no breaking changes, otherwise describe the breaking change, the new behavior, and the migration path; title or commit must use `type!:` -->

## 验证

- [ ] `CGO_ENABLED=1 go test ./...` 通过 / `CGO_ENABLED=1 go test ./...` passes
- [ ] 改了 `internal/delta/`：`go test ./internal/delta/...` 通过 / `go test ./internal/delta/...` passes if changed `internal/delta/`
- [ ] 改了前端：`cd frontend && yarn lint && yarn build` / `cd frontend && yarn lint && yarn build` passes if changed frontend
- [ ] 改了 API：已同步进 `internal/controller/openapi.*.json`，`TestOpenAPIRoutesSync` 与 `TestPlaneSpecsValid` 通过 / `TestOpenAPIRoutesSync` and `TestPlaneSpecsValid` passes and synced to `internal/controller/openapi.*.json` if changed API
- [ ] 改了前端契约：执行 `yarn generate:api`，没有手改 `src/api/generated/` 或 `generated-client/` / `yarn generate:api` executed, no manual changes to `src/api/generated/` or `generated-client/` if changed admin api
- [ ] 没有提交带密钥的 `configs/*.yaml`，也没有把 SDK 源码带进 `main` / No `configs/*.yaml` with secrets committed, and no SDK source code included in `main`
