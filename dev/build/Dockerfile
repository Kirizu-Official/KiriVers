# syntax=docker/dockerfile:1
# Official process image (no Postgres/Redis, no hdiffz/hpatchz).
# Context is the repository root:
#   docker build -f dev/build/Dockerfile .
#
# Local / contributor builds: multi-stage yarn + CGO musl (golang alpine + build-base).
# CI: copy prebuilt kirivers-linux-${TARGETARCH} with
#   docker build --build-arg USE_PREBUILT=1 --target runtime ...

ARG GO_VERSION=1.27
ARG NODE_VERSION=22
ARG ALPINE_VERSION=3.22
ARG USE_PREBUILT=0
# Build-info stamp, same env names as dev/build/kirivers_build/build.py and
# .github/workflows/release.yml, same defaults as that script's _STAMP_VARS.
# Commit is deliberately absent: .dockerignore excludes .git, so a source build
# in this file can never know the revision and internal/buildinfo + the Vite
# config both fall back to "unknown". CI does not use this path at all (it sets
# USE_PREBUILT=1 and COPYs a binary that the Go build job already stamped).
ARG KIRIVERS_VERSION=dev
ARG KIRIVERS_BUILD_TIME=unknown

# --- frontend (skipped by BuildKit when USE_PREBUILT=1) ---
FROM node:${NODE_VERSION}-alpine AS frontend
WORKDIR /src/frontend
RUN corepack enable && corepack prepare yarn@1.22.22 --activate
COPY frontend/package.json frontend/yarn.lock ./
RUN yarn install --frozen-lockfile
# Re-declared here rather than at the top of the stage: a pre-FROM ARG is not
# inherited by a stage, and placing the ENV below the dependency layer keeps a
# version bump from throwing away `yarn install`. ENV (not a yarn argument) is
# what frontend/vite.config.mts reads.
ARG KIRIVERS_VERSION
ARG KIRIVERS_BUILD_TIME
ENV KIRIVERS_VERSION=${KIRIVERS_VERSION}
ENV KIRIVERS_BUILD_TIME=${KIRIVERS_BUILD_TIME}
COPY frontend/ ./
RUN yarn build

# --- CGO musl builder (linux; TARGETARCH is amd64 or arm64) ---
FROM golang:${GO_VERSION}-alpine AS builder
RUN apk add --no-cache build-base ca-certificates git file python3
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/dist ./frontend/dist
ARG TARGETARCH=amd64
ARG KIRIVERS_VERSION
ARG KIRIVERS_BUILD_TIME
# .dockerignore excludes .git; do not let go build fail on VCS stamping.
ENV GOFLAGS=-buildvcs=false
ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=${TARGETARCH}
# -ldflags stays one shell word (quoted expansion), exactly like the literal
# form it replaces; cmd/go splits it into argv itself. Both link attempts carry
# the same stamp, so the dynamic-musl fallback cannot ship an unstamped binary.
# The symbol names are internal/buildinfo's unexported injected vars — lowercase
# `version` / `buildTime`, matching _STAMP_VARS in
# dev/build/kirivers_build/build.py. cmd/link silently ignores a -X target it
# cannot resolve, so a typo here is invisible unless
# internal/buildinfo/buildinfo_test.go is run with the same -X pair.
RUN stamp="-X github.com/Kirizu-Official/KiriVers/internal/buildinfo.version=${KIRIVERS_VERSION} -X github.com/Kirizu-Official/KiriVers/internal/buildinfo.buildTime=${KIRIVERS_BUILD_TIME}" \
	&& go build -trimpath -buildvcs=false -ldflags "-s -w -linkmode external -extldflags \"-static\" ${stamp}" -o /out/kirivers . \
	|| go build -trimpath -buildvcs=false -ldflags "-s -w ${stamp}" -o /out/kirivers .
RUN python3 /src/dev/build/kirivers.py assert-linux-musl /out/kirivers

# --- runtime bits shared by source and prebuilt ---
FROM alpine:${ALPINE_VERSION} AS runtime-base
RUN apk add --no-cache ca-certificates tzdata libstdc++ wget
WORKDIR /var/kirivers
ENV KIRIVERS_CONFIG=/config
ENV KIRIVERS_STORAGE_LOCAL_ROOT=/data/storage
ENV KIRIVERS_LOG_FILE_DIR=/data/logs
ENV KIRIVERS_ADMIN_LOG_SYSTEM_FILE_DIR=/data/logs
ENV KIRIVERS_ADMIN_LOG_ACCESS_FILE_DIR=/data/logs
ENV KIRIVERS_CLIENT_LOG_SYSTEM_FILE_DIR=/data/logs
ENV KIRIVERS_CLIENT_LOG_ACCESS_FILE_DIR=/data/logs
VOLUME ["/config", "/data"]
EXPOSE 8080 8081
# Admin plane default :8081. GET /api/v1/health is HTTP 200 even when ready=false (no DB).
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
	CMD wget -qO- http://127.0.0.1:8081/api/v1/health || exit 1

FROM runtime-base AS runtime-0
COPY --from=builder /out/kirivers /usr/local/bin/kirivers
ENTRYPOINT ["kirivers"]

# CI: place kirivers-linux-amd64 / kirivers-linux-arm64 at the build context root.
FROM runtime-base AS runtime-1
ARG TARGETARCH=amd64
COPY kirivers-linux-${TARGETARCH} /usr/local/bin/kirivers
RUN chmod 0755 /usr/local/bin/kirivers
ENTRYPOINT ["kirivers"]

FROM runtime-${USE_PREBUILT} AS runtime
