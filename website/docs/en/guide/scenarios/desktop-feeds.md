---
title: "Other desktop updaters"
description: "Squirrel RELEASES, ClickOnce .application, AppImage .zsync, WinGet REST, MSIX .appinstaller. Official SDKs do not read these feeds."
---

# Other desktop updaters

These protocols run **beside** native JSON check: the updater fetches the listing document; official SDKs do not read them. Missing listing or unknown document name → **plaintext 404**. OS/arch/channel pins: [Store listings](/en/guide/features/store).

Console: project **Settings → Store listings** → **New listing**.

| Updater | protocol | URL to paste |
|---------|----------|----------------|
| Squirrel | `squirrel` | `{listing}/RELEASES` (case-sensitive `RELEASES`) |
| ClickOnce | `clickonce` | `{listing}/MyApp.application` (must end with `.application`) |
| AppImageUpdate | `appimage` | `{listing}/latest.zsync` (must end with `.zsync`) |
| WinGet REST source | `winget` | listing root; documents `information`, `manifestSearch`, `packageManifests` |
| MSIX / App Installer | `msix` | `{listing}/app.appinstaller` (must end with `.appinstaller`) |

Replace host, project slug, and listing slug with the console **Store URL**.

::: code-group

```text [Squirrel]
https://updates.example.com/api/v1/projects/myapp/store/squirrel/stable/RELEASES
```

```text [ClickOnce]
https://updates.example.com/api/v1/projects/myapp/store/clickonce/stable/MyApp.application
```

```text [AppImage]
https://updates.example.com/api/v1/projects/myapp/store/appimage/stable/latest.zsync
```

```text [WinGet]
https://updates.example.com/api/v1/projects/myapp/store/winget/stable
```

```text [MSIX]
https://updates.example.com/api/v1/projects/myapp/store/msix/stable/app.appinstaller
```

:::

WinGet `PackageIdentifier` lives in listing identifiers. MSIX may override the root Uri with identifiers `appinstaller_uri`. Squirrel RELEASES uses relative filenames; private signed URLs are **not** written into RELEASES rows.

## Failures

Plaintext 404: missing listing, protocol off, or a document name that is not in the table. Auth failure → 401 <ErrorCode code="UNAUTHORIZED" />.

Related: [Electron](./electron), [Sparkle](./sparkle), [Tauri](./tauri).
