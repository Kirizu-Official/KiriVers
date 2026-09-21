---
title: "Delta engines"
description: "hdiffpatch CGO HDIFF13& vs historical KVDIFFHP1. Magic dispatch. C++ toolchain to build."
---

# Delta engines

Single-file binary deltas live in `internal/delta`. The server **must** build with `CGO_ENABLED=1` and a C++ toolchain (Windows MinGW `g++` / Linux `g++` or `clang++` / macOS Xcode `clang++`). It does not exec `hdiffz`/`hpatchz` from PATH at runtime. The Go SDK stays `CGO_ENABLED=0`; see the SDK pages.

| Algorithm | Magic | Notes |
|-----------|-------|-------|
| hdiffpatch (CGO) | `HDIFF13&` | New diffs are uncompressed official HDIFF13; `DescribeAvailable` is `cgo` |
| Historical hdiffpatch | `KVDIFFHP1` | Patch-only; not official `.hdiff` |
| bsdiff | `BSDIFF40` | `pure-go` |
| xdelta3 / VCDIFF | `D6 C3 C4` | RFC 3284 binary magic, not ASCII; `pure-go` |

`Patch` dispatches on magic; never cross-decode. Unknown admin algo → `DELTA_ALGO_UNSUPPORTED`. Admin `POST .../artifacts/delta` enqueues a Job. HTTP handlers must not import `archive/zip`.

Official `darwin/*` binaries must be CGO-built on a macOS host (`darwin/arm64` natively, `darwin/amd64` same-host `clang -arch x86_64`); never a Linux→darwin CGO cross without an Apple SDK. The GitHub release workflow builds both on a pinned macOS runner (`macos-15`). GitHub is the only forge, so no target is ever skipped for lack of a host: if an artifact is missing, the packaging job fails instead of fabricating a file.
