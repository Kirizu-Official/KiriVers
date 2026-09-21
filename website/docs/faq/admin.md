---
title: "管理后台"
description: "登录、2FA、Passkey NOT_READY、LAST_OWNER、上传、COMPARE_ENGINE_IMMUTABLE。"
---

# 管理后台

| 现象 | 说明 |
|------|------|
| 密码对但仍停在第二因素 | pending TTL 过期，重新登录 |
| 无法添加 Passkey | `webauthn_rp_id` 或 `webauthn_origins` 任一为空 → `NOT_READY`；用 TOTP |
| 删除成员失败 | `LAST_OWNER` / `LAST_ADMIN` |
| 发布失败 | `ARTIFACT_REQUIRED`、`UPLOAD_INCOMPLETE`、`AUTO_PUBLISH_PENDING` |
| 改比较引擎 409 | `COMPARE_ENGINE_IMMUTABLE` |
| 上传直传 S3 失败 | `direct_s3` 恒 false，走控制台/TUS |

顶栏铃铛查看 Job。没有独立任务路由。
