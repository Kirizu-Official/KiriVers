---
title: "Install / console not opening"
description: "Which Release archive to download, why an old link 404s, GLIBC errors, checksum verification, DSN, Redis startup Ping, empty static_dir, self-built binary without yarn build."
---

# Install / console not opening

| Symptom | Check |
|---------|-------|
| No idea which archive to download | On the Release page pick by prefix: `KiriVers-<OS>-<Arch>-`, see [Picking a Release asset](/en/guide/install/#releases-files). The extracted executable is named `kirivers-<os>-<arch>` |
| Yesterday's download link 404s today | Expected: the last six characters of the archive name are the binary's content hash, so they change every release. Bookmark the Release page, not an individual file link |
| Linux reports `GLIBC_x.y not found` | The `x86`, `armv7` and `riscv64` archives are cross-built against glibc ≥ 2.39. On older systems switch to [Docker](/en/guide/install/docker) (musl, amd64/arm64 only) or build from source |
| How do I verify a checksum | `sha256sum -c SHA256SUMS.txt` (the first section covers the 11 uploaded files). The second section records the in-archive binaries as `#` comments; extract and compare them yourself |
| Process exits immediately | `postgres.dsn`; whether one of the five log streams has both sinks off; `changelog.default_entries > max_entries` |
| `cache.driver=redis` will not start | Startup Ping failed. Start Redis or switch back to `memory` |
| Wrong port | Client `:8080`, admin `:8081` |
| JSON instead of the login page | `admin.enabled`; `static_dir: ""` disables the UI; a self-built binary that never built the admin console (no embedded `index.html`) |
| Official Release shows no UI | Official binaries already embed the console; copying `frontend/dist` is unnecessary. Check that `static_dir` was not set to the empty string |

On Windows use [manual install](/en/guide/install/manual); the one-click script only supports Linux with apt/yum. `static_dir` is the admin console; the VitePress docs `dist` is not the console.
