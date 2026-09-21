---
title: "Reverse proxy and TLS"
description: "Expose the client plane publicly; bind admin to loopback or a private network. Terminate TLS on the plane YAML or on the proxy."
---

# Reverse proxy and TLS

Publish only the client plane (`:8080`) on the internet. Bind the admin plane to `127.0.0.1` or a private network, then reach it over VPN / SSH / an internal proxy.

TLS options (mixable per plane):

1. Plane YAML `tls_cert` + `tls_key` (both non-empty).
2. Proxy terminates TLS; then that plane must list `trusted_proxies` or `X-Forwarded-For` is ignored.

Empty `trusted_proxies` calls `SetTrustedProxies(nil)` — **not** Gin’s default trust-all.

Production admin reverse-proxies `/api/v1/projects/**` to this process’s client plane for console previews. Admin `GET /` serves the console SPA; host the docs site separately so it does not collide with the SPA.
