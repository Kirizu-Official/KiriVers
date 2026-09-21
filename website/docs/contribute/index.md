---
title: "贡献"
description: "给要改默认分支源码的人。自托管请回指南。Issue → PR。yarn dev 调试 UI。"
---

# 贡献

本区给**修改 KiriVers 默认分支源码**的人。只想安装使用请回 [指南](/guide/)。仓库：<https://github.com/Kirizu-Official/KiriVers>（公开后再提交 Issue / PR）。

| 入口 | 页面 |
|------|------|
| 缺陷 | [提交 BUG](./bugs) |
| 补丁 | [贡献代码](./pull-requests) |
| 树与进程 | [目录结构](./repo-layout)、[运行架构](./architecture) |
| 域模型白皮书 | [技术白皮书](./whitepaper)（改默认分支前：对象分层、唯一实现点、leftover 404） |
| 本地循环 | [本地开发](./dev-setup)、[发版与 Docker](./release) |

顺序是 **Issue → PR**：先开 Issue，再开绑定它的 PR（`Fixes #编号` 或侧栏 Link an issue）；确实不需要 Issue 时给 PR 加 `skip-issue-check` 标签。这道门只卡进 `main` 的 PR。合入 `main` **不会**发布服务端：发版是维护者手动运行的工作流。

日常改管理台 UI：`cd frontend && yarn dev`（`http://localhost:3000`）。生产内嵌才需要 `yarn build`。
