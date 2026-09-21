---
title: "GeoIP"
description: "平台管理员上传 MaxMind 兼容 .mmdb。MaxMind GeoLite2 与 Loyalsoldier Country.mmdb。删除名册不等于隐私擦除。"
---

# GeoIP

抽屉标题 **GeoIP 数据库**。仅**平台管理员**可见。项目成员请求 `/geoip` 得到 403 <ErrorCode code="FORBIDDEN" />。未知库 404 <ErrorCode code="GEOIP_NOT_FOUND" />。

上传的是 MaxMind 兼容 **`.mmdb`**。进程按 **排序（rank）升序** 查询：先 `City` 再 `Country`，融合 ISO `country_code` / `region_code` 与 `Names` 地图。文件写入**私有**存储键 `geoip/{id}/…`，不得进公共桶。仓库不收录 MMDB，在控制台上传即可。

## 谁能打开

1. 用平台管理员登录。
2. 打开全局抽屉 **GeoIP**。

## 取得 `.mmdb`

控制台只接受解压后的 **`.mmdb`**。不要上传 `.tar.gz`、CSV、`.dat`。

::: code-group

```md [MaxMind GeoLite2]
1. 打开 https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/
2. 注册账号并取得 license key。
3. 在账号 Download Files 页下载 **GeoLite2-Country.mmdb**（国家）和/或 **GeoLite2-City.mmdb**（国家 + 行政区，用于 region_code）。
4. 若得到 `.tar.gz`，解压后只上传其中的 `.mmdb`。
5. 遵守 GeoLite [EULA](https://www.maxmind.com/en/geolite2/eula)，保持库相对新。ASN 库对控制台国家图不是必需。
```

```md [Loyalsoldier]
1. 打开 https://github.com/Loyalsoldier/geoip/releases
2. 下载 **Country.mmdb**（MaxMind 格式）。
3. 可选：`Country-without-asn.mmdb`、`Country-asn.mmdb`、`Country-only-cn-private.mmdb`（偏 CN/私网，不是完整世界 Country 库）。
4. **不要**上传 `geoip.dat` 或其它 `*.dat`（V2Ray/Xray），也不是 Clash/Surge 规则集。非 `.mmdb` → <ErrorCode code="INVALID_REQUEST" />。
```

:::

## 上传

页内说明：「上传 MaxMind 兼容 .mmdb，按 rank 融合查询。最多 8 个，每个不超过 256 MiB。」文件提示：「仅接受 .mmdb。不会把授权库提交进仓库。」

| 字段 | 界面 | 含义 |
|------|------|------|
| 名称 | **名称** | 管理台显示名 |
| 文件 | **文件** | 必须是 `.mmdb` |
| 排序 | **排序** | 数值越小越先查询（**上移** / **下移**） |

1. 点 **上传 MMDB**。
2. 填写 **名称**，选择 `.mmdb` 文件，设置 **排序**。
3. 确认上传。上限：**最多 8 份**、每份 **256 MiB**。超限或非 `.mmdb` → <ErrorCode code="INVALID_REQUEST" />。

::: warning 私有存储
`storage.driver=s3` 且未配置 `storage.private` 桶时上传失败，**不得**把 GeoIP 写入公共桶。本机 `storage.driver=local` 时与产物共用磁盘后端，但仍用 `geoip/{id}/…` 键隔离。见 [远程存储](/guide/config/s3)。
:::

Lookup 在内存中完成。进程按对象 Head 跳过未变化的库。名册里的国家/地区来自上报时的解析结果。

## 删除库 ≠ 隐私擦除

点 **删除 GeoIP 库** 只卸载这份 MMDB 读者。删除 **客户端** 名册行也**不是**隐私擦除。按设备哈希擦除在项目设置 **隐私** 面板，响应含 `telemetry_deleted` / `allowlist_deleted`。见 [设备列表](/admin/projects/clients) 与 [遥测与设备](/guide/features/telemetry)。
