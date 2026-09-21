---
title: "Web / Node"
description: "KiriVers ships installable objects. Refreshing a page is not a site upgrade. Browser apps cannot use the TypeScript SDK."
---

# Web / Node

KiriVers distributes **installable objects** (SHA-256 files). Reloading a web page is not a site-upgrade channel.

## When to use it

- Site static assets / SPA builds: your own CDN / pipeline, not KiriVers check.
- A **Node server process** in the same product (daemon, CLI): native JSON **POST** check.
- A desktop shell (Electron) wrapping a web UI: the installer uses an [Electron](./electron) listing or a homegrown client inside the shell.

The official TypeScript SDK has **no** browser export and cannot call check in a web page.

## Where to configure

Node processes: project **Settings** rate limits / tokens, plus [Update check](/en/guide/features/check). Electron shells: **Settings → Store listings** → **New listing**.

## Client / updater duties

| Shape | Approach |
|-------|----------|
| In-browser web app | Do not use the TypeScript SDK. Ship site assets with your own CDN / build |
| Node server process | **POST** check as in [Servers and CLI](./server-cli) |
| Electron shell | [Electron](./electron) store listing, or a homegrown native JSON client inside the shell |

## Failures

Importing the official SDK in a browser fails (no browser build). Uploading site HTML as an “installer” does not make a page reload run check. Node processes must **POST**; GET check is 404.

Related: [Update check](/en/guide/features/check), [SDK](/en/api/sdk/).
