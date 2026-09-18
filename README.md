# kirizu/kirivers-client

本分支是**跳转**，不含 Composer 包源码。Packagist 索引的是独立仓库根上的 `composer.json`，不能指向本仓 `main` 或本分支。

- 发布仓：https://github.com/Kirizu-Official/KiriVers-SDK-PHP  
  （Composer：`kirizu/kirivers-client`）
- 本仓完整源码（开发与测试）：`sdk/php-src`

```text
composer require kirizu/kirivers-client
```

独立仓尚未创建时，从 `sdk/php-src` 拷到新仓根目录再打 tag。步骤见默认分支 [`docs/sdk-publish.md`](https://github.com/Kirizu-Official/KiriVers/blob/main/docs/sdk-publish.md)。
