---
layout: home
title: "KiriVers"
description: "Self-hosted software update service: publish artifacts; apps check and download updates."
hero:
  name: KiriVers
  text: Self-hosted software updates
  tagline: Publish artifacts. Applications check for updates and download them. Official binaries embed the admin UI.
  image:
    src: /logo.svg
    alt: KiriVers
  actions:
    - theme: brand
      text: Install
      link: /en/guide/install/
    - theme: alt
      text: Admin quick start
      link: /en/admin/quick-start
    - theme: alt
      text: SDK
      link: /en/api/sdk/
features:
  - title: Dual plane
    details: One process listens on the client plane (default :8080) and the admin plane (default :8081).
  - title: Native JSON and store feeds
    details: First-party clients POST an update check. Electron / Sparkle and similar updaters consume store listing URLs.
  - title: PostgreSQL required
    details: Redis is optional. With cache.driver=redis, startup Ping must succeed or the process will not listen.
---
