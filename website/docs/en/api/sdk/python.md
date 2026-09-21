---
title: "Python SDK"
description: "kirivers-client. Client / Config / Updater. Runtime deps: requests + cryptography."
---

# Python

## 1. Install

```bash
pip install kirivers-client
```

Python 3.10+. Runtime: `requests` and `cryptography` only.

::: warning Not on PyPI yet
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

## 3. Download and verify

```python
pkg = client.download_url(check.update.package_url)
# compare pkg.body to check.update.sha256
```

Keep `exp`/`sig`. Support Range.

## 4. Updater

```python
result = Updater(client).run(
    current_version="1.0.0", os="windows", arch="x86_64", channel="stable",
    device_id="your-stable-device-id", dest_path="downloads/app.bin", apply=False,
)
```

`apply=False` stages only. Without a Replacer that can replace a busy file, this is not one-click install.

## 5. Client methods

health, project, report, check, changelog, integrity, diff, pack (same-URL poll), packages GET/HEAD, channels/matrix/languages, announcements, telemetry (202), media. Not implemented: `/store/`, GET `/update/check`, `POST .../clients/login`, `GET /api/v1/ready`, `GET .../manifest`, `POST /update/pack/status`, `/artifacts/{id}/{filename}`.

## 6. Config and auth

`Config(base_url, project_ref)` plus optional project token, channel token, verify PEM. Do not log tokens. The app generates `device_id`.

## 7. Adapters

| Adapter | Default | Inject when |
|---------|---------|-------------|
| Transport | requests | tests / custom HTTP |
| JSON | stdlib json (not an adapter) | — |
| Hasher | hashlib | |
| FileStore | pathlib/os + NFC | |
| ArchiveUnpacker | zipfile | disable to omit patch_package |
| SignatureVerifier | cryptography Ed25519+RSA | |
| Replacer | os.replace | busy destination raises |
| Patcher | **none** | required for binary_delta |

Do not add httpx.

## 8. Capabilities

Default check: `full_package` only. Updater adds `file_list` / `patch_package` / `binary_delta` from live adapters.

## 9. Patcher

Inject `supported_algos` + `apply`. Magics: `KVDIFFHP1` / `HDIFF13&` / `BSDIFF40` / VCDIFF. Unknown → full package. No default Patcher.

## 10. Replacer / platforms

Default `os.replace`. A locked Windows exe raises; inject a Replacer. Android/HarmonyOS APK and iOS IPA are caller-owned. Signed macOS app replace is the caller’s.

## 11. Errors

Read `{error.code}`. 204/304 are not exceptions. Unknown codes stay opaque.

## 12. Transport / tests

Inject a fake Transport. Contract tests do not use Docker.

## 13. Language notes

Default Replacer is `os.replace` and raises on a busy target. No default Patcher.
