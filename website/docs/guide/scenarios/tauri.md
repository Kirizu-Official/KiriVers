---
title: "Tauri"
description: "protocol=tauri。静态 latest.json 或动态 {target}/{arch}/{current_version}。官方 SDK 不读该 feed。"
---

# Tauri

Tauri v2 updater 读取 JSON feed。后台 protocol 为 `tauri`。官方 SDK 只封装原生 JSON check，**不**读取 `latest.json`。

## 产物形态

按平台发布安装器（`.msi` / `.dmg` 等）。feed 只投影默认硬件变体；协议无法表达 `hw_rev`。签名字段为 minisign 容器（与原生 check 的裸签名字节格式不同）。RSA 项目或未配私钥时省略 `signature`。

两种端点：

| 形态 | 路径 | 行为 |
|------|------|------|
| 静态 | `.../store/tauri/{listing}/latest.json` | 多平台 `platforms` 对象。可带 `os` / `arch` / `channel` query；省略时覆盖矩阵中 Tauri 可用组合 |
| 动态 | `.../store/tauri/{listing}/{target}/{arch}/{current_version}` | 有更新 200 单平台对象；无更新 **204** 无 body。`target` ∈ `darwin` \| `windows` \| `linux`（接受 `macos` 别名） |

未知 `target` / `arch` 或非上述路径 → 纯文本 404。

## 控制台

1. **设置 → 商店协议** → **新建上架**，协议 `tauri`。
2. 复制 **商店 URL**。动态端点再拼 `{{target}}/{{arch}}/{{current_version}}`。
3. 客户端公钥登记在 listing identifiers 的 `public_key`（本服务不替客户端验签）。

## 更新器配置

::: code-group

```json [动态 endpoints]
{
  "plugins": {
    "updater": {
      "endpoints": [
        "https://updates.example.com/api/v1/projects/myapp/store/tauri/stable/{{target}}/{{arch}}/{{current_version}}"
      ],
      "pubkey": "<minisign-public-key>"
    }
  }
}
```

```json [静态 latest.json]
{
  "plugins": {
    "updater": {
      "endpoints": [
        "https://updates.example.com/api/v1/projects/myapp/store/tauri/stable/latest.json"
      ],
      "pubkey": "<minisign-public-key>"
    }
  }
}
```

:::

真实 Tauri 客户端请求静态文档时通常**不带** os/arch query。

## 故障

| 现象 | 原因 |
|------|------|
| 纯文本 **404** | listing 缺失、开关关闭、未知路径 |
| HTTP **204** | 动态端点：已是最新 |
| HTTP 401 <ErrorCode code="UNAUTHORIZED" /> | feed Token 失败 |

相关：[商店订阅](/guide/features/store)、[检查更新](/guide/features/check)。
