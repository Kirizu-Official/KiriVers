---
title: "PHP SDK"
description: "composer require kirizu/kirivers-client. Dedicated repo KiriVers-SDK-PHP. No Guzzle. Not the server repo root."
---

# PHP

PHP 8.2+. Composer `kirizu/kirivers-client`. Publish repo **KiriVers-SDK-PHP**. `require` is `php` + `ext-json` only. HTTP is curl or fopen. **Do not install Guzzle. Do not FFI-patch.**

## 1. Install

```bash
composer require kirizu/kirivers-client
```

::: warning Packagist / dedicated GitHub repository not public yet
```bash
git clone -b sdk/php-src https://github.com/Kirizu-Official/KiriVers.git KiriVers-SDK-PHP
```
Do not `composer require` this server repo `main` or pointer branch `sdk/php`.
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

Class `Kirizu\KiriVers\Client\Client`. Do not `composer require` the server repository.

## 3. Download and verify

`$client->download()` keeps `exp`/`sig` and compares SHA-256.

## 4. Updater

`Updater::withDefaults($client)->run(new UpdateRequest(...))`. No `installPath` → stage only.

## 5. Client methods

`project()`, `deviceReport()`, `check()`, changelog/integrity/diff/pack, download/head, catalogs, announcements, `report()`, media, `health()`. Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename.

## 6. Config and auth

`Config(baseUrl, projectRef, projectToken, channelToken)`. Do not log tokens.

## 7. Adapters

Transport: ext-curl else fopen. FileStore filesystem. Hasher `hash()`. JSON ext-json. ZipArchive if ext-zip. SignatureVerifier sodium/openssl. Patcher **interface only**. Replacer `rename()`.

## 8. Capabilities

`Updater::withDefaults()` sends `full_package` plus `file_list`/`patch_package` when zip exists. No Patcher → no `binary_delta`.

## 9. Patcher

No FFI, no default `hpatchz` exec. Do not cross-decode magics.

## 10. Replacer

Default `rename()` is not a busy-exe / APK installer.

## 11. Errors

Parse `error.code`. 204/304 are success.

## 12. Transport / tests

Inject Transport. `composer test`. No Docker.

## 13. Language notes

`ext-json` required; HTTP is curl or fopen; no Guzzle; no FFI patching.
