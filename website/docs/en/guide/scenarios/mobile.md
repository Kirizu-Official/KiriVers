---
title: "Mobile apps"
description: "Android sideload is a single file. F-Droid is a listing. This does not replace App Store / Play review."
---

# Mobile apps

## Package shape

Android **sideload** publishes the APK as a **single file** (matrix single-file). Official SDKs are not APK installers: you must supply a Replacer. Compare and download remain **POST** check plus a SHA-256 object.

An F-Droid subscription is a separate store listing (`fdroid`), not the native JSON URL. Index documents:

::: code-group

```text [index-v1]
https://updates.example.com/api/v1/projects/myapp/store/fdroid/stable/index-v1.json
```

```text [index-v2]
https://updates.example.com/api/v1/projects/myapp/store/fdroid/stable/index-v2.json
```

:::

Console: **Settings → Store listings** → **New listing**, protocol `fdroid`. Unknown paths (for example `index.xml`) are plaintext 404. Official SDKs do **not** read the F-Droid index.

iPhone enterprise / sideload can use native JSON or a custom drop; that does **not** replace App Store review. Play / App Store private protocols are out of scope.

## Failures

Missing listing → plaintext 404. Native check codes: [FAQ · Error codes](/en/faq/errors).

Related: [Store listings](/en/guide/features/store), [Desktop](./desktop).
