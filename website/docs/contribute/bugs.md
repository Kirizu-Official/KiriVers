---
title: "提交 BUG"
description: "GitHub Issues。填写复现步骤、期望与实际、版本、平面、打码日志。先开 Issue，再开绑定它的 PR。"
---

# 提交 BUG

打开 <https://github.com/Kirizu-Official/KiriVers/issues/new>。Issue 表单模板在仓库 `.github/ISSUE_TEMPLATE/`。

建议填写：

1. 可复现步骤
2. 期望与实际
3. 二进制版本或 commit（Release 包名 `KiriVers-<OS>-<Arch>-<sha6>.zip` 末尾那六位可以唯一定位那次发版；自编译请注明 commit）
4. 操作系统
5. 访问的是客户端平面（`:8080`）还是管理平面（`:8081`）
6. 相关日志（打码密钥、Token、DSN 密码）

安全类漏洞不要开公开 Issue。仓库公开后见 SECURITY；在此之前私下联系维护者。

## 顺序：先 Issue，再 PR

Issue 是流程的起点，修复它的 PR 必须回指这个 Issue：

1. 先开 Issue，把复现信息写全（上面那六项）。
2. 确认要改后，从默认分支拉功能分支，开 PR 并绑定该 Issue：侧栏 **Development → Link an issue**，或在 PR 描述里写 `Fixes #<编号>`（`Closes` / `Resolves` 同样有效）。
3. 确实不需要 Issue 的 PR（例如维护者自己的整理改动）加 `skip-issue-check` 标签。

这道门只卡**进 `main` 的 PR**；守卫 bot 的判定与豁免见 [贡献代码](/contribute/pull-requests)。
