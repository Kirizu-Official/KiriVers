---
title: "备份与升级"
description: "备份 PostgreSQL、对象存储或 local.root、三份 YAML、日志。替换二进制；无独立 migrate CLI。"
---

# 备份与升级

## 备份清单

| 数据 | 位置 | 说明 |
|------|------|------|
| 元数据 | PostgreSQL | `pg_dump`；含项目、版本、渠道、会话相关表 |
| 产物 | `storage.local.root` 或 S3/R2 桶 | 安装包、差量、清单 |
| 私有对象 | `storage.private` | GeoIP MMDB |
| 配置 | 三份 YAML | 不含进 git 的密钥 |
| 日志 | 各 `log.file.dir` | 可选 |

恢复时先恢复数据库与对象，再启动同一版本或更新后的二进制。进程启动执行 GORM AutoMigrate，**没有**独立 `kirivers migrate` 命令。

## 升级二进制

1. 备份上表。
2. 停进程（systemd / 任务计划 / `docker stop`）。
3. 替换 Releases 可执行文件或拉取新镜像。官方产物已含管理台，不必再拷前端。
4. 启动；看 `GET /api/v1/health`：HTTP 200 且 `ready=true`（DB + 存储 Head `.ready`）。

跨大版本前阅读该次 Release 说明。`admin.yaml` 的 `static_dir` 指向管理台静态文件，不是文档站目录。
