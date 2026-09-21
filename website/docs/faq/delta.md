---
title: "下载与差量"
description: "magic 交叉解码、DELTA_ALGO_UNSUPPORTED、Job 失败、打包回退整包。"
---

# 下载与差量

不得交叉解码：`KVDIFFHP1`、`HDIFF13&`、`BSDIFF40`、VCDIFF `D6 C3 C4`。未知 magic 回退整包。管理侧未知算法 → `DELTA_ALGO_UNSUPPORTED`。后台差量 Job 失败见铃铛 / `JOB_FAILED`。

多文件 pack 过大为 **200** `full_package`，不是 400。客户端 pack **无**管理 `job_id`。`local_sha256` 只出现在 diff。下载路径是 `/packages/{sha256}`，不是 `/artifacts/{id}/{filename}`。
