---
title: "Python SDK"
description: "kirivers-client。Client / Config / Updater。依赖仅 requests + cryptography。"
---

# Python

## 1. 安装

```bash
pip install kirivers-client
```

Python 3.10+。运行时仅 `requests` 与 `cryptography`。

::: warning 尚未出现在 PyPI
```bash
git clone -b sdk/python https://github.com/Kirizu-Official/KiriVers.git kirivers-client-python
cd kirivers-client-python
pip install .
```
:::

## 2. Quick start

```python
from kirivers_client import Client, Config, Updater

client = Client(Config(base_url="http://127.0.0.1:8080", project_ref="my-app"))
check = client.check(
    current_version="1.0.0", os="windows", arch="x86_64", channel="stable",
    device_id="your-stable-device-id",
)
if check.no_update or check.not_modified:
    print("up to date")  # HTTP 204 / 304
elif check.update:
    print(check.update.package_url)
```

## 3. 下载并核对

```python
pkg = client.download_url(check.update.package_url)
# 比对 pkg.body 与 check.update.sha256
```

保留 URL 上的 `exp`/`sig`，支持 Range。

## 4. Updater

```python
result = Updater(client).run(
    current_version="1.0.0", os="windows", arch="x86_64", channel="stable",
    device_id="your-stable-device-id", dest_path="downloads/app.bin", apply=False,
)
```

`apply=False` 只暂存。未注入能处理占用文件的 Replacer 时不要当成一键安装。

## 5. Client 方法

health、project、report、check、changelog、integrity、diff、pack（同 URL 轮询）、packages GET/HEAD、channels/matrix/languages、announcements、telemetry（202）、media。不实现：`/store/`、GET `/update/check`、`POST .../clients/login`、`GET /api/v1/ready`、`GET .../manifest`、`POST /update/pack/status`、`/artifacts/{id}/{filename}`。

## 6. 配置与鉴权

`Config(base_url, project_ref)`，可选项目令牌、渠道令牌、验签 PEM。令牌不要打进日志。`device_id` 由应用生成。

## 7. 适配器

| 适配器 | 默认 | 何时注入 |
|--------|------|----------|
| Transport | requests | 测试 / 自定义 HTTP |
| JSON | stdlib json（不是适配器） | — |
| Hasher | hashlib | |
| FileStore | pathlib/os + NFC | |
| ArchiveUnpacker | zipfile | 关掉则不报 patch_package |
| SignatureVerifier | cryptography Ed25519+RSA | |
| Replacer | os.replace | 占用目标会抛给调用方 |
| Patcher | **无** | 才能报 binary_delta |

不要再加 httpx/axios。

## 8. 能力位

默认 check 仅 `full_package`。Updater 按活着的适配器加 `file_list` / `patch_package` / `binary_delta`。

## 9. Patcher

注入 `supported_algos` + `apply`。magic：`KVDIFFHP1` / `HDIFF13&` / `BSDIFF40` / VCDIFF。未知回退整包。无默认 Patcher。

## 10. Replacer 与平台

默认 `os.replace`。占用中的 Windows exe 会抛错，须自备 Replacer。Android/HarmonyOS APK、iOS IPA 均须自备。macOS 签名应用替换是调用方责任。

## 11. 错误

解析 `{error.code}`。204/304 不是异常。未知 code 保持原字符串。

## 12. Transport / 测试

注入假 Transport。契约测试不依赖 Docker。

## 13. 语言特有

默认 Replacer 是 `os.replace`，占用目标抛给调用方。无默认 Patcher。
