---
title: "PHP SDK"
description: "composer require kirizu/kirivers-client。独立仓 KiriVers-SDK-PHP。不要 Guzzle。不要把主仓当包根。"
---

# PHP

PHP 8.2+。Composer `kirizu/kirivers-client`。发布仓 **KiriVers-SDK-PHP**。`require` 仅 `php` + `ext-json`。HTTP 为 curl 或 fopen。**不要装 Guzzle。不要 FFI 打补丁。**

## 1. 安装

```bash
composer require kirizu/kirivers-client
```

::: warning Packagist / 独立仓尚未公开
```bash
git clone -b sdk/php-src https://github.com/Kirizu-Official/KiriVers.git KiriVers-SDK-PHP
```
不要 `composer require` 本仓 `main` 或跳转分支 `sdk/php`。
:::

## 2. Quick start

```php
use Kirizu\KiriVers\Client\Client;
use Kirizu\KiriVers\Client\Config;

$client = new Client(new Config(
    baseUrl: 'http://127.0.0.1:8080',
    projectRef: 'my-app',
));
$check = $client->check([
    'current_version' => '1.0.0',
    'os' => 'windows',
    'arch' => 'x86_64',
    'channel' => 'stable',
    'device_id' => $callerOwnedDeviceId,
    'capabilities' => ['full_package'],
]);
if ($check->noUpdate || $check->notModified) {
    return; // HTTP 204 / 304
}
```

类名 `Kirizu\KiriVers\Client\Client`。不要 `composer require` 本服务器仓库。

## 3. 下载并核对

`$client->download()` 保留 `exp`/`sig`，比对 SHA-256。

## 4. Updater

`Updater::withDefaults($client)->run(new UpdateRequest(...))`。无 `installPath` 只暂存。

## 5. Client 方法

`project()`、`deviceReport()`、`check()`、`changelog()`、`integrity()`、`diff()`、`pack()` / `packUntilReady()`、download/head、catalogs、`announcements()`、`report()`、media、`health()`。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename。

## 6. 配置与鉴权

`Config(baseUrl, projectRef, projectToken, channelToken)`。令牌不要写入日志。

## 7. 适配器

Transport：ext-curl 否则 fopen。FileStore 文件系统。Hasher `hash()`。JSON ext-json。ZipArchive 若有 ext-zip。SignatureVerifier sodium/openssl。Patcher **仅接口**。Replacer `rename()`。

## 8. 能力位

`Updater::withDefaults()` 会报 `full_package` 加 `file_list`/`patch_package`（若 zip 存在）。无 Patcher 无 `binary_delta`。

## 9. Patcher

不要 FFI、不要默认 shell 出 `hpatchz`。magic 不交叉解码。

## 10. Replacer

默认 `rename()` 不是占用 exe / APK 安装器。

## 11. 错误

解析 `error.code`。204/304 不是失败。

## 12. Transport / 测试

注入 Transport。`composer test`。不依赖 Docker。

## 13. 语言特有

`ext-json` 必需；HTTP 为 curl 或 fopen；不要 Guzzle；不要 FFI 打补丁。
