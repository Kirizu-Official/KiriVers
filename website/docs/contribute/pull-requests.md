---
title: "贡献代码"
description: "功能分支 PR：Conventional Commits 标题、必须绑定 Issue、自动版本号与更新日志、同步 OpenAPI、不要手改 generated。"
---

# 贡献代码

从默认分支拉功能分支，向 <https://github.com/Kirizu-Official/KiriVers> 开 PR。

## PR 标题就是版本号来源

合入 `main` 的 squash 提交信息取自 PR 标题，而标题决定这次合入是否升版本：

| 类型 | 档位 |
|------|------|
| `feat` | minor |
| `fix` / `perf` / `revert` | patch |
| `type!:` 或正文 `BREAKING CHANGE:` | major |
| `docs` / `refactor` / `test` / `style` / `chore` / `ci` / `build` | 不发版 |

写法：`<类型>(<可选 scope>): <一句话说明>`，例如 `fix(client): 修正空响应`。可用类型：feat | fix | perf | revert | docs | refactor | test | style | chore | ci | build。

- **scope 用小写**（`perf(db)`，不是 `perf(DB)`）。它同时是更新日志里条目的归属标注。
- 破坏性变更写成 `类型!:` 或在正文加 `BREAKING CHANGE:` 脚注。
- PR 标题与每个提交都要合规（`python3 dev/build/kirivers.py commitlint`）。不合规的 squash 标题会使检查失败。

## PR 必须绑定 Issue

每个进 `main` 的 Pull Request 都要关联一个 Issue，两种方式任选：

- 用右侧栏 **Development → Link an issue**，或
- 在 PR 描述里写一行 `Fixes #<编号>`（`Closes` / `Resolves` 同样有效，任意大小写）。

确实不需要 Issue 时，给这个 PR 加 `skip-issue-check` 标签。

范围说明：**只卡 `main` 的 PR**。直推 `main`（维护者）与 `sdk/<语言>` 分支的 PR 不受这道门约束——SDK 分支的发版由该分支自己的流水线负责。

### 守卫 bot 会做什么

`.github/workflows/pr-guard.yml` 在 PR 打开、改标题、推新提交、重开、从 draft 转为可评审（ready for review）时复检标题格式与 Issue 关联，并在 PR 里维护**一条**留言（不会重复刷屏）：

- 外部贡献者（非组织成员）未通过检查 → 留言并**自动关闭** PR。处理完之后点 **Reopen pull request** 重新打开即可，守卫会自动复检并把那条留言更新为通过提示；不必重开一个新的 PR。
- 组织成员（OWNER / MEMBER / COLLABORATOR）未通过 → 只留言提醒，不关闭。
- **draft PR 与 bot 账号完全豁免**（还没写完的东西不该被关）。

守卫只读 PR 元数据，不检出、不执行 PR 带进来的任何代码，也不构建或测试。

先开 Issue 再开 PR 的顺序见 [报告缺陷](/contribute/bugs)。

## 更新日志自动生成，不要手写

`CHANGELOG.md` 与 GitHub Release 正文都由发版脚本从 commit 生成（格式与可自定义的 emoji/文案见 [发版与 Docker](/contribute/release#changelog-format)）。手工编辑 `CHANGELOG.md` 会在下一次发版被覆盖式插入挤乱，且不会带来版本号变化。章节由 `type` 决定、条目里的 `**scope**` 由你的 scope 决定——把 scope 写对，日志就自动归位。

## 其他要求

- 相关 `go test` / 前端 lint 通过。
- 改 API 时同步该平面 `internal/controller/openapi.*.json`，并跑 `TestOpenAPIRoutesSync` 与 `TestPlaneSpecsValid`。
- 改前端契约后在 `frontend/` 执行 `yarn generate:api`。不要手改 `src/api/generated/` 或 `generated-client/`。
- 不要把各语言 SDK 源码推进 `main`（它们在孤儿分支 `sdk/<语言>` 上，见 [客户端 SDK](/contribute/sdk)）。
- 不要提交带密钥的 `configs/*.yaml`（只用 `*-example.yaml`）。
- 合入 `main` **不会**触发服务端发布：发版是维护者手动运行的工作流。

## 本地自测

```bash
python3 dev/build/kirivers.py check-workflows      # 流水线静态门
python3 dev/build/kirivers.py guard --self-check   # 守卫判定矩阵
python3 dev/build/kirivers.py issue-link --self-check
CGO_ENABLED=1 go test ./...
cd frontend && yarn lint && yarn test
```

Windows 上把 `python3` 换成 `python`。
