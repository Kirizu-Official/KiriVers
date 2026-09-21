---
title: "发版与 Docker"
description: "手动触发的服务端发版、Conventional Commits 版本推导、十平台矩阵与 libc 策略、更新日志格式与自定义、Docker 多架构标签。"
---

# 发版与 Docker

维护者把进程镜像和官方二进制从默认分支发出去。使用者安装说明在 [指南 → 安装](/guide/install/)。本页只写仓库里的流水线与本地 `dev/build`。发版工具链的唯一入口是 `dev/build/kirivers.py`：Linux / macOS 上 `python3 dev/build/kirivers.py <子命令>`，Windows 上 `python dev/build/kirivers.py <子命令>`（纯标准库，无需安装）。下文只写子命令名。

GitHub 是唯一的托管端（forge）：检查、发版、镜像都在 GitHub Actions，仓库里没有第二套流水线。

## 路径

| 用途 | 路径 |
|------|------|
| 运行镜像（Alpine，无数据库） | `dev/build/Dockerfile`，构建上下文为仓库根 |
| 一键运行（Hub 镜像 + Postgres + Redis） | `deploy/`；口令在 `deploy/.env`（使用者文档，见 [Docker Compose](/guide/install/compose)） |
| 发版 CLI | `dev/build/kirivers.py` + 包 `dev/build/kirivers_build/`（纯标准库 Python；工作流只做编排，判定都在 CLI 里） |
| GitHub PR 检查 | `.github/workflows/ci.yml`（`pull_request` + `push` → `main`） |
| PR 守卫 | `.github/workflows/pr-guard.yml`（标题与 Issue 绑定，见 [贡献代码](/contribute/pull-requests)） |
| 安全门禁 | `.github/workflows/gosec-scan.yml`（`wait-merge` 标签触发只读沙箱 Gosec 扫描）+ `.github/workflows/security-gate.yml`（扫描通过后自动晋升 `ready-merge`；新提交即时撤销 `wait-merge` / `ready-merge`） |
| GitHub 发版 | `.github/workflows/release.yml`（**只** `workflow_dispatch`，不 `on: push`、不 `on: tags`） |
| 更新日志类型表 | `dev/build/changelog-types.conf`（显示层，不参与版本推导） |
| 发版追加的日志 | 仓库根 `CHANGELOG.md`（由 `dev/build/kirivers.py changelog` 写回 `main`） |
| 官方 SDK 发版 | 各 `sdk/<语言>` 分支自带的 `.github/workflows/`；维护者也可用默认分支的 `sdk-automerge.yml`（**只** `workflow_dispatch`）批量合并 `ready-merge` PR 并在同一运行内推送各语言 registry，见 `docs/sdk-publish.md` |

不要在仓库根添加 `compose.yml` / `docker-compose.yml`。镜像内不安装 `hdiffz` / `hpatchz`，官方 HDIFF13 由进程内 CGO 产出。

## 触发：合入 main 不会发布

服务端发版是**手动独占**的：把 PR 合进 `main` 只跑检查。要发版，去 Actions → Release → **Run workflow**。

- 版本号自动推导，**没有**手填版本的入口，也没有"强制升一级"的开关。
- 区间内没有可发版提交（只有 `chore:` / `docs:` / `ci:` 等）时，本次运行**绿色结束**，不打 tag、不建 Release、不推镜像，并在 job summary 里写明原因。
- `SERVER_PUBLISH` 是手动 kill-switch：设为 `false` 时即使有可发版提交也只推导版本，不产出任何外部副作用。

## 版本（与 SDK 同构，tag 前缀不同）

服务端 tag 是打在 `main` 祖先上的 annotated `v<semver>`（例如 `v0.1.0`）。查找上一版本时忽略 `sdk-*-v*`。

1. 没有服务端 `v*` → 发布基线 **`0.1.0`**（即使该提交是 `ci:` / `chore:`），不回放历史 `feat`。
2. 已有 `v*` → `git log <prev>..HEAD`（去掉 merge）：`feat` → minor；`fix` / `perf` / `revert` → patch；正文 `BREAKING CHANGE:` 或 `type!:` → major。`chore` / `docs` / `ci` / `test` / `style` / `refactor` **不发版**。
3. breaking 从 `0.1.x` 跳到 `1.0.0`。
4. 纯 `docs:` 合入 `main`：检查通过，且手动运行时不发版、不打新 tag、不推 Hub。
5. 仓库没有服务端 VERSION 文件，不提交 `chore(release)`。

实现：`next-version` 子命令（`dev/build/kirivers_build/version.py`）。PR 用 `commitlint` 校验标题与提交；直推 `main` 时改测 `before..after` 区间（`before` 全零或不可达时退化为只检最新提交）。不合规的 squash 标题会使检查失败。

## 平台矩阵：十种组合，零 QEMU

一律 `yarn build` 之后 **`CGO_ENABLED=1`**。`CGO_ENABLED=0` 不是发版路径（CI 会断言它编译失败）。

| 目标 | libc | 宿主 | 构建方式 |
|------|------|------|----------|
| linux/amd64 | **musl** | `ubuntu-latest` | Alpine 容器内原生 gcc/g++ |
| linux/arm64 | **musl** | `ubuntu-24.04-arm` | 同上，arm64 原生宿主 |
| linux/386 | glibc | `ubuntu-latest` | amd64 宿主交叉：`g++-i686-linux-gnu` |
| linux/arm (armv7) | glibc | `ubuntu-latest` | amd64 宿主交叉：`g++-arm-linux-gnueabihf` |
| linux/riscv64 | glibc | `ubuntu-latest` | amd64 宿主交叉：`g++-riscv64-linux-gnu` |
| darwin/amd64 | — | `macos-15` | Apple clang `-arch x86_64` |
| darwin/arm64 | — | `macos-15` | Apple clang 原生 |
| windows/386 | — | `windows-latest` | llvm-mingw `i686-w64-mingw32` |
| windows/amd64 | — | `windows-latest` | llvm-mingw `x86_64-w64-mingw32` |
| windows/arm64 | — | `windows-latest` | llvm-mingw `aarch64-w64-mingw32` |

- **全程不使用 QEMU 用户态模拟**：musl 只在能原生产出的架构上用，其余 Linux 架构退化为 glibc 交叉编译。代价写进[手动安装](/guide/install/manual)：glibc 产物要求目标机 glibc ≥ 2.39，armv7 需 hard-float。
- 交叉编译器静默回落宿主是最危险的失败模式，因此 `build-cgo` 每个目标编完用 `file` 断言机器类型，打包时在 Linux 作业里对十个产物再断言一次。
- darwin 产物只在 macOS 宿主上产出，**绝不**从 Linux 伪造。
- 编译作业 `fail-fast: false`；但缺件时 `package` 与 `github-release` 必须失败，宁可不发也不发半个版本。

## 产物：归档名带内容哈希

内部二进制名保持稳定（`kirivers-<os>-<arch>[.exe]`），上传的是每目标一个 zip：

```
KiriVers-<OS>-<Arch>-<sha6>.zip
KiriVers-Linux-x86_64-449c6d.zip     KiriVers-macOS-arm64-ad1a24.zip
KiriVers-Linux-x86-108619.zip        KiriVers-Windows-x86-635890.zip
KiriVers-Linux-armv7-ec6a8c.zip      KiriVers-Windows-x86_64-abb5fc.zip
KiriVers-Linux-arm64-0c690f.zip      KiriVers-Windows-arm64-0f98c4.zip
KiriVers-Linux-riscv64-074587.zip
frontend-dist.zip                    SHA256SUMS.txt
```

`<sha6>` = 包内二进制 SHA-256 的前 6 位小写十六进制（文件无法自哈希，所以文件名每次发版都会变）。OS token 用 `Linux` / `macOS` / `Windows`，Arch token 用 `x86` / `x86_64` / `armv7` / `arm64` / `riscv64`。映射表只定义在 `dev/build/kirivers_build/common.py`，`asset-names` 与本页表格都由它派生。

`SHA256SUMS.txt` 两段式：第一段是 11 个上传件自身（可直接 `sha256sum -c SHA256SUMS.txt`），第二段以注释记录 10 个包内二进制的哈希（`#` 前缀保证不被 `-c` 解析），解压后可自行比对。

`frontend-dist.zip` 给"只想换管理台静态文件"的场景：包内是 `dist/` 的内容，根目录就是 `index.html`。官方二进制已内嵌管理台，使用者不需要它。

## 更新日志：按 type 分节，scope 做前缀 {#changelog-format}

Release 正文与仓库根 `CHANGELOG.md` 由同一个 `release-notes` 子命令（`dev/build/kirivers_build/notes.py`）输出生成，格式与 SDK 分支的 `scripts/sdk_release.py` 一致：

```markdown
## v0.3.0 — 2026-10-05

### 💥 Breaking Changes
- **admin** feat: 重写 API Token 轮换，旧接口下线 (8a9b0c1) #31

### ✨ Features
- **delta**: 支持流式源文件 (4f5e6d7) #28
- 直推且没有 scope 的 feat 照进本节 (6bc7e20)

### 🐛 Bug Fixes
- **delta**: 拒绝截断的 HDIFF13 头部 (a1b2c3d) #29
```

- 章节 = commit 的 `type`；标题 = `### <emoji> <文案>`，emoji、文案与**章节顺序**都取自类型表（文件里的书写顺序即章节顺序）。
- 条目 = `- **<scope>**: <说明> (<sha7>) #<PR>`。`scope` 统一转小写并去空白（`perf(DB)` 与 `perf(db)` 同一个标注）；没有 scope 时省略 `**scope**:` 前缀，不塞进任何兜底章节。
- 破坏性条目（`type!:` 或正文 `BREAKING CHANGE:` / `BREAKING-CHANGE:`）离开自己的 type 节，集中到首节 `### 💥`，并在行内补回 type 词（因为章节标题已不携带它）。
- PR 号是**装饰**：作业用 `gh api .../commits/<sha>/pulls` 查询，查不到、`gh` 缺失或离线时静默省略，绝不因此中断发版；说明末尾已有 `(#123)` 时不重复追加。
- 非 Conventional 格式的 subject 不进日志（与"不发版"档位一致）；未登记的合法 type 落 `### 🔖 Other`，**不会静默丢条目**；区间内一条可显示条目都没有时输出 `No user-facing Conventional Commits in this range.`。
- 没有前序 `v*` tag 时只写基线声明，不回放历史，不分章节。

### 自定义 emoji / 文案 / 隐藏某类

唯一入口是 `dev/build/changelog-types.conf`（SDK 分支为 `scripts/changelog-types.conf`，内容相同）。下面是节选，真实文件还带一段说明注释以及 `test` / `style` / `ci` / `build` 等默认注释掉的行：

```ini
breaking=💥=Breaking Changes
feat=✨=Features
fix=🐛=Bug Fixes
perf=⚡=Performance
revert=⏪=Reverts
refactor=♻️=Refactoring
# docs=📝=Documentation
# chore=🔧=Chores
```

一行一项 `type=emoji[=章节文案]`（`type=emoji 文案` 也接受）；整行以 `#` 开头且形如 `# type=…` 表示**该类不进更新日志**，其余 `#` 行是说明文字。格式非法（键不是小写字母开头、缺 emoji、同一个 type 写两遍）直接报错退出，不静默忽略。文件缺失时脚本回落到内置默认表，并断言"解析结果与文件一致"，防止两份默认漂移。

::: danger 这条不变量请守住
类型表**只是显示层**。`next-version` 的档位表与 `sdk_release.py` 的 `RELEASE_TYPES` 绝不读它——注释掉 `fix=` 只会让 Bug Fixes 章节消失，patch 升档照旧。回归断言在 `release-notes --self-check`：改配置前后 `next-version --format env` 的输出必须逐字节相同。
:::

## CHANGELOG.md 写回 main

`github-release` 作业在建 tag、发 Release **之后**追加 `CHANGELOG.md` 头部并 `git push origin HEAD:main`，提交信息 `docs(release): v<semver> changelog`（`docs:` 不计入下一次发版，不会自造版本差）。因此该提交不属于被发布的 commit 集合，也不在 `<semver>` tag 的祖先里。

这要求仓库允许 `github-actions[bot]` 用 `GITHUB_TOKEN` 写 `main`（与 SDK 分支放行 bot 合并同构）。若分支保护没放开，该步骤**失败并说明原因**，不会静默跳过。

## Docker：每架构原生构建 + 远端合成 manifest

| 标签 | 含义 |
|------|------|
| `kirizuofficial/kirivers:latest` | 指向本次发版 |
| `kirizuofficial/kirivers:<semver>` | 本次发版的多架构 manifest |
| `kirizuofficial/kirivers:<semver>_amd64` | 该版本的 amd64 单架构镜像 |
| `kirizuofficial/kirivers:<semver>_arm64` | 该版本的 arm64 单架构镜像 |

两个架构各在**原生 runner** 上用 `docker/login-action@v3` + `docker/build-push-action@v6`（`--build-arg USE_PREBUILT=1` 复用已编好的 musl 二进制）推每架构标签；再由 `docker-manifest` 作业用 `buildx imagetools create` 在注册表侧合成 `:latest` 与 `:<semver>`。`imagetools create` 不执行镜像内任何命令，因此不需要 QEMU/binfmt。glibc 产物**绝不**进 Alpine 镜像：每架构作业只 COPY `linux-musl` 矩阵（musl 断言通过）的产物；本地手工路径 `docker-image` 同样只接受 musl 断言通过的二进制，且要求宿主架构与目标一致。

作业顺序保证只有两个每架构标签都推送成功才合成 manifest——合成错误标签会污染 `:latest`。

## 密钥与关闭 publish

文档只列名称，不写值：

| 名称 | 用途 |
|------|------|
| `DOCKERHUB_USERNAME` | Docker Hub 用户 |
| `DOCKERHUB_TOKEN` | Docker Hub 登录 |
| `SERVER_PUBLISH` | GitHub repository variable；`false` 时本次运行只推导版本 |
| `GITHUB_TOKEN` | 工作流自带；写 tag、Release、`CHANGELOG.md`，读 PR 号 |

缺 Hub 凭据时**推镜像步骤**失败，并说明缺 `DOCKERHUB_USERNAME` 和/或 `DOCKERHUB_TOKEN`；日志不得打印 token。并发组名：`server-release`。

## 本地构建镜像

```bash
docker build --platform linux/amd64 -t kirivers:linux-amd64 -f dev/build/Dockerfile .
```

日常运行请用 Hub 镜像：使用者一键目录 `deploy/`（`image: kirizuofficial/kirivers:latest`，见 [Docker Compose](/guide/install/compose)）。本机 `docker build` 只用于改 Dockerfile 之后再推 Hub。镜像默认 `KIRIVERS_STORAGE_LOCAL_ROOT=/data/storage`，日志目录 `/data/logs`。把宿主机目录挂到 `/config` 与 `/data` 即可持久化配置、全量/差量对象和日志。

HEALTHCHECK 访问管理平面 `GET /api/v1/health`（无库仍 HTTP 200）。容器以 root 运行以便 bind mount。

## 必测项

改动这条链路后至少跑：

```bash
python3 dev/build/kirivers.py check-workflows        # 静态门：触发方式、矩阵、无 QEMU、无其它 forge 残留
python3 dev/build/kirivers.py release-notes --self-check
python3 dev/build/kirivers.py guard --self-check
python3 dev/build/kirivers.py issue-link --self-check
CGO_ENABLED=1 go build . && CGO_ENABLED=0 go build .   # 后者必须失败
```

Windows 上把 `python3` 换成 `python`。`check-workflows` 已经把"只有 `workflow_dispatch`""没有 `setup-qemu-action`""`pr-guard.yml` 不检出 PR 代码"等写成断言，CI 的 `changelog` 作业会跑它。
