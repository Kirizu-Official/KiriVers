---
title: "服务与命令行程序"
description: "无 UI 进程：启动时 POST check，按运维节奏轮询。产品不规定固定间隔。"
---

# 服务与命令行程序

## 产物形态

无界面二进制（Linux 服务、Windows 服务、CLI）。按单文件或多文件目录发布，见 [桌面应用](./desktop)。商店 feed 通常不适用。

## 客户端职责

1. 进程启动时 **POST** check。
2. 之后按运维节奏周期查询。产品**不**规定固定秒数。响应带 `Retry-After` 时必须遵守。
3. 未注入 Replacer 时：把已校验文件放到暂存路径，由 systemd / 任务计划 / 编排系统替换正在运行的二进制。
4. 遥测 `POST .../telemetry/report` 可选；失败不得阻断更新。

相关：[检查更新](/guide/features/check)、[网站 / Node](./web-node)。
