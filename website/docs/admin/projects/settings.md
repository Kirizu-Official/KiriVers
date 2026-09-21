---
title: "项目设置"
description: "与 settings.vue 面板对齐：基本信息到清理、成员。安装策略有专页。"
---

# 项目设置

导航 **设置**。页说明：「项目策略、存储、Webhook、隐私与成员。」点 **保存** 提交 PATCH。安装策略规则不在本 JSON 里，见 [安装策略](./install-policy)。

1. **进入项目** → **设置**。
2. 按下面面板修改，每块改完点 **保存**（或该面板自己的确认按钮）。

## 基本信息

| 字段 | 界面 | 何时改 | 失败 |
|------|------|--------|------|
| Slug | **Slug** | 公开标识要换名 | 过期别名后客户端 <ErrorCode code="PROJECT_NOT_FOUND" /> |
| 别名保留期（天） | **别名保留期（天）** | 旧 slug 还要解析多久 | 非法值 400 <ErrorCode code="INVALID_REQUEST" /> |
| 比较引擎 | **比较引擎** | 仅首次发布前 | <ErrorCode code="COMPARE_ENGINE_IMMUTABLE" /> |
| 最低支持版本 | **最低支持版本** | 强制过旧客户端升级 | 按项目比较引擎比较 |
| 默认语言 | **默认语言** | 缺文案回退 | 须是已登记语言 |

## 更新策略

打开 **更新策略**。

- **灰度放号优先老设备且近期活跃**：默认开。关则按创建时间、id 升序。
- **file_list 待下载上限**：`0` 继承实例天花板，不得超过输入框 max，否则 400 <ErrorCode code="INVALID_REQUEST" />。

## 安装策略模板

面板标题 **安装策略模板**。点进去编辑规则，见 [安装策略](./install-policy)。

## 安全

- **客户端检查需要 Token**：开启后客户端须带项目 Token，否则 401 <ErrorCode code="UNAUTHORIZED" />。
- **商店 Token**：**生成** 或手填后仅本次保存展示明文；GET 只返回 `has_store_token`。**清除商店 Token** 取消鉴权。
- **强制 HTTPS** / **CORS 允许来源（逗号分隔）**。

## 存储

**存储可见性** 固定 public。界面提示 private 会被接口拒绝（400 <ErrorCode code="INVALID_REQUEST" />）。**存储前缀** / **存储桶** 不得与项目 slug 相同。签名算法与公钥用于 check 响应验签。

## 商店协议

信封 `{ listings: [] }`。

1. 点 **新建上架**。
2. 填 **协议**（`electron`、`sparkle`、`tauri` 等，与后端 `Protocol()` 一致）、**Listing slug**、可选钉死 OS/Arch/渠道、**完整包来源**。
3. 复制 **商店 URL** 给更新器。重复 `(protocol, slug)` → 400 <ErrorCode code="INVALID_REQUEST" />。删除后对应 URL **纯文本 404**（无 JSON `code`）。

概念：[商店订阅](/guide/features/store)。

## Webhook

填 **Webhook URL**。事件：版本发布、版本线就绪、版本吊销。点 **查看投递记录**。密钥不要写进文档示例。见 [Webhook](/guide/features/webhooks)。

## 更新日志默认值

范围、排版、**无 from 时条数** / **区间截断上限** 受实例 `changelog.*` 约束。超过天花板 400 <ErrorCode code="INVALID_REQUEST" />。见 [更新说明](/guide/features/changelog)。

## 速率限制

面板 **速率限制**。客户端超限 429 <ErrorCode code="RATE_LIMITED" />，带 `Retry-After`。`store_per_ip_per_minute` 是共享的公开 GET IP 桶。

## 隐私

**device_id 策略**、**遥测留存（天）**、**按哈希删除设备**。擦除返回 `telemetry_deleted` / `allowlist_deleted`。删除名册行 ≠ 本操作。见 [设备列表](./clients)。

## 清理旧包

**产物清理**：填 **留存期（天）**，确认「确定执行产物清理？被清理的产物不可恢复。」永不删除原生全量包与商店 `store_full`。进度看顶栏 **任务中心**。失败 <ErrorCode code="JOB_FAILED" />。

## 项目成员

1. **添加成员**，选 **角色**（**拥有者** / **管理员**）。
2. **切换角色** 或 **移除成员**。不能移除最后一名拥有者：<ErrorCode code="LAST_OWNER" />。未知：<ErrorCode code="MEMBER_NOT_FOUND" />。
