"""Build-chain subcommands: the CGO/musl compilers and their toolchain installers.

Ported one-for-one from `dev/build/build-cgo.sh`, `build-linux-musl.sh`,
`assert-linux-musl.sh`, `install-cross-toolchains.sh`, `install-llvm-mingw.sh`
and `frontend-build.sh`. The contracts that keep the workflows working are the
eval-captured CC/CXX lines (C1), the single GITHUB_PATH line (C2), exit codes
(C11) and "diagnostics on stderr, captured payload on stdout" (C12); each is
annotated at the point it is honoured.

Paths passed as arguments follow the shell originals: the build commands ran
with the repo root as their working directory, so a relative `<outfile>` lands
under the root, while `assert-linux-musl` never moved and resolves against the
caller's directory.
"""

from __future__ import annotations

import fnmatch
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path, PurePosixPath

from . import common
from .common import FAIL_EXIT, ROOT, USAGE_EXIT, asset_name, host_goarch, host_os, is_musl_host, log

# One argv, embedded double quotes included: cmd/go splits this value itself, so
# it must never be handed to a shell or list-ified (design §5.4).
LDFLAGS_RELEASE = "-s -w"
LDFLAGS_STATIC = '-s -w -linkmode external -extldflags "-static"'

# --- build-info injection (-X) --------------------------------------------- #
# internal/buildinfo's package path as Go sees it. It must match `go.mod`'s
# module line: cmd/link silently ignores a -X target it cannot resolve, so a
# typo here is only caught by the injected-values case in
# internal/buildinfo/buildinfo_test.go (see implement.md B5 step 2).
_BUILDINFO_PKG = "github.com/Kirizu-Official/KiriVers/internal/buildinfo"

# (Go symbol, source env var, fallback when the env var is unset). Order is the
# emitted flag order, so two runs on one host produce one ldflags string.
# commit deliberately defaults to empty: "no value" means "do not inject a -X
# at all" and internal/buildinfo falls back to the VCS stamp / unknown. The
# injected value may be a full 40-char SHA (release.yml injects `github.sha`
# because the Actions expression language has no string-slicing function), so
# every commit value passes through _short_commit() below.
_STAMP_VARS = (
    ("version", "KIRIVERS_VERSION", "dev"),
    ("commit", "KIRIVERS_BUILD_COMMIT", ""),
    ("buildTime", "KIRIVERS_BUILD_TIME", "unknown"),
)

# The single width every KiriVers build reports for its commit. It mirrors
# `commitShortLen` in internal/buildinfo/buildinfo.go, which does the same trim
# to a `debug.ReadBuildInfo()` VCS stamp; `git rev-parse --short` picks its own
# width from the object count, so this constant -- not git -- is what keeps an
# injected SHA and a local build the same shape.
_COMMIT_SHORT_LEN = 7

# cmd/go splits the -ldflags value on whitespace and honours its own quote
# syntax, so an injected value carrying a space, a quote or an "=" would split
# into two argv entries and silently corrupt the flags. Everything a real
# version / SHA / RFC3339 timestamp can contain is inside this class; anything
# outside it fails the build instead of shipping a polluted binary.
_STAMP_VALUE_RE = re.compile(r"^[A-Za-z0-9._:/+,-]*$")

ALPINE_PROBE_IMAGE = "alpine:3.22"

# GOOS/GOARCH pairs build-cgo may ship; identical to the release matrix rows, so
# the matrix stays the single source of truth instead of a second hand-tuned list.
_CGO_TARGETS = frozenset(f"{row[0]}/{row[1]}" for row in common.MATRIX)

# CC/CXX defaults, in the original `case` arm order. The first match wins, and a
# caller-supplied CC/CXX is never overwritten (each default is applied per var).
_CC_DEFAULTS = (
    ("linux/386", "i686-linux-gnu-gcc", "i686-linux-gnu-g++"),
    ("linux/arm", "arm-linux-gnueabihf-gcc", "arm-linux-gnueabihf-g++"),
    ("linux/riscv64", "riscv64-linux-gnu-gcc", "riscv64-linux-gnu-g++"),
    ("linux/*", "gcc", "g++"),
    # One env value holding command + args; spaces must survive unsplittable.
    ("darwin/amd64", "clang -arch x86_64", "clang++ -arch x86_64"),
    ("darwin/*", "clang", "clang++"),
    ("windows/386", "i686-w64-mingw32-gcc", "i686-w64-mingw32-g++"),
    ("windows/amd64", "x86_64-w64-mingw32-gcc", "x86_64-w64-mingw32-g++"),
    ("windows/*", "aarch64-w64-mingw32-gcc", "aarch64-w64-mingw32-g++"),
)


def _at_root(raw: str) -> Path:
    """Interpret an argument path the way the shell did after `cd "$ROOT"`."""
    return Path(raw) if Path(raw).is_absolute() else ROOT / raw


def _tool(name: str) -> str:
    """Resolved executable for a `command -v` gate, so the child is findable on
    Windows too (bare `yarn`/`corepack` are `.cmd` shims CreateProcess won't run)."""
    return shutil.which(name) or name


def _go_build(ldflags: str, out: str, env: dict[str, str]) -> int:
    """`go build` one target; `-ldflags` stays a single argv (see LDFLAGS_STATIC)."""
    return common.run(
        ["go", "build", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", out, "."],
        cwd=ROOT,
        env=env,
    )


def _git_short_commit() -> str:
    """Best-effort `git rev-parse --short HEAD` for a local (non-CI) build.

    Empty on any failure (no git binary, no repository, detached-but-unusable
    ref): that is the signal to inject nothing and let internal/buildinfo fall
    back. Output is captured, never printed, because stdout of a subcommand a
    workflow evals is a wire format (C12).
    """
    if shutil.which("git") is None:
        return ""
    proc = subprocess.run(
        ["git", "rev-parse", "--short", "HEAD"],
        cwd=ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        check=False,
    )
    return proc.stdout.strip() if proc.returncode == 0 else ""


def _short_commit(revision: str) -> str:
    """Trim a revision to _COMMIT_SHORT_LEN, the width internal/buildinfo uses."""
    return revision[:_COMMIT_SHORT_LEN]


def _stamp_flags() -> str | None:
    """Build the `-X …` tail from KIRIVERS_{VERSION,BUILD_COMMIT,BUILD_TIME}.

    Returns None when a value cannot be injected safely; the caller must then
    fail the build (see _STAMP_VALUE_RE for why this is fatal, not cosmetic).
    """
    parts: list[str] = []
    for go_name, env_name, default in _STAMP_VARS:
        value = os.environ.get(env_name) or default
        if not value and go_name == "commit":
            value = _git_short_commit()
        if not value:
            # Nothing to say for this field: omit the -X rather than inject an
            # empty string, so the Go-side fallback chain stays in charge.
            continue
        if not _STAMP_VALUE_RE.match(value):
            log(
                f"refusing to inject {env_name}={value!r}: it must match "
                "[A-Za-z0-9._:/+,-]* (no spaces, quotes or '='), because "
                "cmd/go splits the -ldflags value on whitespace"
            )
            return None
        # Checked before the trim, never after: shortening first would let a
        # value whose offending character sits past the cut-through slip through
        # the guard above.
        if go_name == "commit":
            value = _short_commit(value)
        parts.append(f"-X {_BUILDINFO_PKG}.{go_name}={value}")
    return " ".join(parts)


def _ldflags(base: str) -> str | None:
    """Append the build-info `-X` injections to one of the LDFLAGS_* constants.

    The constants themselves stay untouched, and the return value is still a
    single -ldflags argv element (it is passed to `_go_build` as one list item
    and never through a shell). None means an injected value was rejected.
    """
    tail = _stamp_flags()
    if tail is None:
        return None
    return f"{base} {tail}" if tail else base


# --------------------------------------------------------------------------- #
# build-cgo
# --------------------------------------------------------------------------- #


def build_cgo(argv: list[str]) -> int:
    """`build-cgo <goos> <goarch> <outfile>` — one CGO_ENABLED=1 target binary."""
    if len(argv) != 3:
        log("usage: kirivers.py build-cgo <goos> <goarch> <outfile>")
        return USAGE_EXIT
    goos, goarch, out = argv

    # Only the literal "0" is rejected; "" behaves like unset (`${CGO_ENABLED:-1}`)
    # and any other value is overwritten with 1 below, exactly as the shell did.
    if (os.environ.get("CGO_ENABLED") or "1") == "0":
        log("CGO_ENABLED=0 is not a release path")
        return FAIL_EXIT

    if f"{goos}/{goarch}" not in _CGO_TARGETS:
        log(f"unsupported {goos}/{goarch}")
        return USAGE_EXIT

    # Resolve the injection before touching the filesystem: a rejected stamp
    # value must leave no artifact behind (AC3).
    ldflags = _ldflags(LDFLAGS_RELEASE)
    if ldflags is None:
        return FAIL_EXIT

    _at_root(out).parent.mkdir(parents=True, exist_ok=True)

    cc = os.environ.get("CC") or ""
    cxx = os.environ.get("CXX") or ""
    goarm = ""
    if goarch == "arm":
        # armv7 hard-float is the only linux/arm build we ship.
        goarm = os.environ.get("GOARM") or "7"
    if not cc or not cxx:
        key = f"{goos}/{goarch}"
        for pattern, cc_default, cxx_default in _CC_DEFAULTS:
            # fnmatchcase: shell `case` globs are case-sensitive, and Windows
            # fnmatch would otherwise fold the pattern to lower case.
            if fnmatch.fnmatchcase(key, pattern):
                cc = cc or cc_default
                cxx = cxx or cxx_default
                break

    # Inherited environment is passed through untouched (GOFLAGS, GOPROXY, ...);
    # only these keys are set, like the original `export` lines.
    env = dict(os.environ)
    env["CGO_ENABLED"] = "1"
    env["GOOS"] = goos
    env["GOARCH"] = goarch
    env["CC"] = cc
    env["CXX"] = cxx
    if goarm:
        env["GOARM"] = goarm

    goarm_note = f" GOARM={goarm}" if goarm else ""
    log(f"building {out} (CGO_ENABLED=1 GOOS={goos} GOARCH={goarch}{goarm_note} CC={cc})")
    status = _go_build(ldflags, out, env)
    if status:
        return status
    # A cross compiler that silently fell back to the host toolchain would produce
    # a runnable-looking but wrong-architecture binary; assert the machine type.
    return common.assert_machine(goos, goarch, _at_root(out))


# --------------------------------------------------------------------------- #
# assert-linux-musl
# --------------------------------------------------------------------------- #

# Case-sensitive on purpose (plain `grep -Eq`): the alternation lists the case
# variants itself. The machine-type patterns in common.py are the loose,
# case-insensitive counterpart — the two gates must not be unified.
_GLIBC_RE = re.compile(r"ld-linux|libc\.so\.6|GLIBC|glibc")
_MUSL_RE = re.compile(
    r"ld-musl|musl|statically linked|static-pie|not a dynamic executable|Not a valid dynamic program"
)


def _assert_one_musl(display: str, binary: Path) -> int:
    if not binary.is_file():
        log(f"assert-linux-musl: missing {display}")
        return FAIL_EXIT

    info = ""
    target = binary.as_posix()  # Keep slashes as the shell saw them, on any host.
    if shutil.which("file"):
        # `file -b` first; on failure retry with the name-preferring form.
        proc = subprocess.run(["file", "-b", target], capture_output=True, text=True, check=False)
        if proc.returncode:
            proc = subprocess.run(["file", target], capture_output=True, text=True, check=False)
        info = proc.stdout.rstrip("\n")

    if shutil.which("ldd") is None:
        log("assert-linux-musl: ldd is required to assert musl/static (missing on this host)")
        return FAIL_EXIT
    # ldd's stderr is folded into its stdout (`ldd ... 2>&1`) and a non-zero exit
    # is tolerated: busybox/glibc ldd disagree loudly about static binaries.
    proc = subprocess.run(
        ["ldd", target], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, check=False
    )
    ldd_out = proc.stdout.rstrip("\n")
    blob = f"{info}\n{ldd_out}"

    if _GLIBC_RE.search(blob):
        log(f"assert-linux-musl: {display} looks glibc-linked:\n{blob}")
        return FAIL_EXIT
    if _MUSL_RE.search(blob):
        log(f"ok musl/static: {display}")
        return 0
    log(f"assert-linux-musl: {display} is not musl/static (refusing unknown libc):\n{blob}")
    return FAIL_EXIT


def assert_linux_musl(argv: list[str]) -> int:
    """`assert-linux-musl <binary>...` — refuse anything glibc-linked."""
    if not argv:
        log("usage: kirivers.py assert-linux-musl <binary>...")
        return USAGE_EXIT
    for raw in argv:
        # Standalone in the shell version too: paths belong to the caller's CWD.
        status = _assert_one_musl(raw, Path(raw))
        if status:
            return status
    return 0


# --------------------------------------------------------------------------- #
# build-linux-musl
# --------------------------------------------------------------------------- #


def build_linux_musl(argv: list[str]) -> int:
    """`build-linux-musl <amd64|arm64> [outfile]` — Alpine libc, never host glibc."""
    if not argv:
        log("usage: kirivers.py build-linux-musl <amd64|arm64> [outfile]")
        return USAGE_EXIT
    arch = argv[0]
    if arch not in ("amd64", "arm64"):
        log("usage: kirivers.py build-linux-musl <amd64|arm64> [outfile]")
        return USAGE_EXIT
    out = argv[1] if len(argv) > 1 else f"dist/{asset_name('linux', arch)}"

    # The volume fallback does `docker cp <cid>:/src/<out>`, and the in-container
    # re-invocation joins `<out>` under /src: an absolute path would silently
    # produce no artifact, so it is refused rather than guessed at. Both path
    # flavours are checked, because the container speaks POSIX while the host may
    # be Windows: `Path("/tmp/x").is_absolute()` is False under Git Bash, which
    # would let a MSYS-style absolute path through and lose the artifact.
    if PurePosixPath(out).is_absolute() or Path(out).is_absolute():
        log(f"build-linux-musl: <outfile> must be relative to the repo root, got: {out}")
        return USAGE_EXIT

    image = common.golang_alpine_image(ROOT)
    native = is_musl_host() and host_goarch() == arch

    if native:
        return _musl_native(arch, out)

    if shutil.which("docker") is None:
        log(f"need docker to build linux/{arch} musl from this host")
        return FAIL_EXIT
    # Fail before starting a container: the payload re-enters this CLI inside
    # Alpine, so an unusable stamp value would otherwise only surface minutes
    # later (and after an image pull).
    if _stamp_flags() is None:
        return FAIL_EXIT
    _at_root(out).parent.mkdir(parents=True, exist_ok=True)
    status = _musl_docker(arch, out, image)
    if status:
        return status
    # Host-side gate: fully static binaries read `statically linked`, musl-dynamic
    # ones read `ld-musl`, so glibc hosts can still assert the artifact.
    return _assert_one_musl(out, _at_root(out))


def _musl_native(arch: str, out: str) -> int:
    if shutil.which("gcc") is None:
        log("musl host needs gcc/g++ (apk add build-base)")
        return FAIL_EXIT
    # Both link attempts carry the same injection, so the dynamic-musl fallback
    # cannot silently ship an unstamped binary when the static link fails.
    static_ldflags = _ldflags(LDFLAGS_STATIC)
    release_ldflags = _ldflags(LDFLAGS_RELEASE)
    if static_ldflags is None or release_ldflags is None:
        return FAIL_EXIT
    env = dict(os.environ)
    env.update({"CGO_ENABLED": "1", "GOOS": "linux", "GOARCH": arch, "CC": "gcc", "CXX": "g++"})
    _at_root(out).parent.mkdir(parents=True, exist_ok=True)
    log(f"linux musl native build -> {out} (trying static)")
    if _go_build(static_ldflags, out, env) != 0:
        # A *caught* failure: a hard error here would kill musl releases whose
        # libstdc++ cannot be linked statically.
        log("static musl link failed; falling back to dynamic musl + libstdc++")
        status = _go_build(release_ldflags, out, env)
        if status:
            return status
    # Returning here is also what terminates the container self-recursion.
    return _assert_one_musl(out, _at_root(out))


def _musl_docker(arch: str, out: str, image: str) -> int:
    platform = f"linux/{arch}"
    # D2: the payload re-enters this CLI inside Alpine, so python3 must be
    # installed with the toolchain bits. `apk` noise stays off the contract stream.
    inner = (
        "apk add --no-cache build-base git ca-certificates file python3 >/dev/null && "
        f'python3 ./dev/build/kirivers.py build-linux-musl "{arch}" "{out}"'
    )

    def build_run(mount: str) -> list[str]:
        return [
            "docker", "run", "--rm", "--platform", platform,
            "-v", mount, "-w", "/src", "-e", "CGO_ENABLED=1", "-e", "GOFLAGS=-buildvcs=false",
            # Forward the build-info stamp by name (unset on the host stays unset
            # in the container). -buildvcs=false means VCS stamping is off, so
            # without these the two Alpine binaries would be the only official
            # artifacts reporting dev/unknown while every other target carried the
            # release version.
            "-e", "KIRIVERS_VERSION", "-e", "KIRIVERS_BUILD_COMMIT", "-e", "KIRIVERS_BUILD_TIME",
            image, "sh", "-c", inner,
        ]

    # Probe: dind / separate-daemon-filesystem hosts cannot see the workspace,
    # which the bind-mount run would report as a confusingly late failure.
    probe = subprocess.run(
        ["docker", "run", "--rm", "-v", f"{ROOT}:/src:ro", ALPINE_PROBE_IMAGE, "test", "-f", "/src/go.mod"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
    )
    if probe.returncode == 0:
        log(f"linux musl docker bind {image} --platform {platform} -> {out}")
        return common.run(build_run(f"{ROOT}:/src"), cwd=ROOT)

    log(f"linux musl docker volume (no host bind) {image} --platform {platform} -> {out}")
    volume = f"kirivers-musl-{os.getpid()}"
    container = ""

    def cleanup() -> None:
        # Idempotent, one step per resource, clearing its handle first: the shell
        # version cleared each variable before touching the next so a retry could
        # not double-delete. Failures here are always swallowed.
        nonlocal container, volume
        if container:
            _quiet(["docker", "rm", "-f", container])
            container = ""
        if volume:
            _quiet(["docker", "volume", "rm", volume])
            volume = ""

    try:
        status = _quiet(["docker", "volume", "create", volume], keep_stderr=True)
        if status:
            return status
        container, status = _docker_cid(["docker", "create", "-v", f"{volume}:/src", ALPINE_PROBE_IMAGE, "true"])
        if status:
            return status
        status = common.run(["docker", "cp", f"{ROOT}/.", f"{container}:/src/"], cwd=ROOT)
        if status:
            return status
        # Keep the handle until the removal really succeeded: if `docker rm` fails,
        # cleanup() still owns the container and force-removes it (the shell's trap
        # did exactly this by clearing musl_cid only after a successful rm).
        status = common.run(["docker", "rm", container], cwd=ROOT)
        if status:
            return status
        container = ""
        status = common.run(build_run(f"{volume}:/src"), cwd=ROOT)
        if status:
            return status
        container, status = _docker_cid(["docker", "create", "-v", f"{volume}:/src", ALPINE_PROBE_IMAGE, "true"])
        if status:
            return status
        status = common.run(["docker", "cp", f"{container}:/src/{out}", out], cwd=ROOT)
        if status:
            return status
        status = common.run(["docker", "rm", container], cwd=ROOT)
        if status:
            return status
        container = ""
        return status
    finally:
        cleanup()


def _quiet(command: list[str], keep_stderr: bool = False) -> int:
    """Run a docker housekeeping child; its stdout never reaches our stdout."""
    proc = subprocess.run(
        command,
        stdout=subprocess.DEVNULL,
        stderr=None if keep_stderr else subprocess.DEVNULL,
        check=False,
    )
    return proc.returncode


def _docker_cid(command: list[str]) -> tuple[str, int]:
    """Capture the container id the way `musl_cid=$(docker create ...)` did."""
    proc = subprocess.run(
        command, stdout=subprocess.PIPE, stderr=sys.stderr, text=True, check=False
    )
    return proc.stdout.strip(), proc.returncode


# --------------------------------------------------------------------------- #
# install-cross-toolchains
# --------------------------------------------------------------------------- #

_TRIPLETS = {
    "386": "i686-linux-gnu",
    "arm": "arm-linux-gnueabihf",
    "riscv64": "riscv64-linux-gnu",
}


def install_cross_toolchains(argv: list[str]) -> int:
    """`install-cross-toolchains <386|arm|riscv64>` — apt glibc cross compilers.

    stdout is `eval`ed by release.yml (C1): exactly the two assignment lines and
    nothing else (D1) — every apt/sudo child is wired to stderr.
    """
    triplet = _TRIPLETS.get(argv[0] if argv else "")
    if triplet is None:
        log("usage: kirivers.py install-cross-toolchains <386|arm|riscv64>")
        return USAGE_EXIT

    sysname = host_os()
    if sysname != "Linux":
        log(f"cross toolchains are installed on Linux runners only (host {sysname})")
        return FAIL_EXIT

    def sudo_sh(script: str) -> int:
        # The payload is a shell string on purpose: apt package lists are word
        # split by the shell, as they were in the original.
        if os.getuid() == 0:
            return common.run(["sh", "-c", script])
        return common.run(["sudo", "sh", "-c", script])

    def apt_install(packages: str) -> int:
        return sudo_sh(
            f"DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends {packages}"
        )

    if shutil.which("apt-get") is None:
        log(f"apt-get is required to install g++-{triplet}")
        return FAIL_EXIT

    pkgset = f"gcc-{triplet} g++-{triplet} libc6-dev-{triplet}-cross"
    if shutil.which(f"{triplet}-g++") is None:
        status = sudo_sh("apt-get update")
        if status:
            return status
        # The -cross multiarch package name differs across Ubuntu releases; fall
        # back to the plain compiler pair when it is unavailable.
        if apt_install(pkgset) != 0:
            log(f"retrying without libc6-dev-{triplet}-cross")
            status = apt_install(f"gcc-{triplet} g++-{triplet}")
            if status:
                return status

    cc_bin = f"{triplet}-gcc"
    cxx_bin = f"{triplet}-g++"
    for tool in (cc_bin, cxx_bin):
        if shutil.which(tool) is None:
            log(f"missing {tool} after installing {pkgset}")
            return FAIL_EXIT

    # Explicit LF write: these two lines are consumed by `eval` (C1).
    sys.stdout.write(f"CC={cc_bin}\nCXX={cxx_bin}\n")
    return 0


# --------------------------------------------------------------------------- #
# install-llvm-mingw
# --------------------------------------------------------------------------- #

LLVM_MINGW_VERSION = "20260616"
_MINGW_GCC_NAMES = ("x86_64-w64-mingw32-gcc", "x86_64-w64-mingw32-gcc.exe")
# uname -s spellings that mean "Windows-ish shell", case-sensitive like `case`.
_MINGW_UNAME_GLOBS = ("MINGW*", "MSYS*", "CYGWIN*", "Windows_NT")


def _from_msys(raw: str) -> str:
    """Git Bash exports HOME=/c/Users/...; native children need C:/Users/...."""
    if os.name == "nt":
        match = re.match(r"^/([a-zA-Z])/(.*)$", raw)
        if match:
            return f"{match.group(1).upper()}:/{match.group(2)}"
    return raw


def install_llvm_mingw(argv: list[str]) -> int:
    """`install-llvm-mingw` — pinned UCRT cross toolchain, bin dir on stdout.

    release.yml appends stdout straight to $GITHUB_PATH (C2): exactly one line.
    """
    version = os.environ.get("LLVM_MINGW_VERSION") or LLVM_MINGW_VERSION
    prefix = Path(
        _from_msys(
            os.environ.get("LLVM_MINGW_PREFIX")
            or f"{_from_msys(os.environ.get('HOME') or '/tmp')}/.cache/llvm-mingw"
        )
    )
    prefix.mkdir(parents=True, exist_ok=True)

    sysname = host_os()
    base = f"https://github.com/mstorsjo/llvm-mingw/releases/download/{version}"
    if sysname == "Linux":
        suffix = "aarch64" if host_goarch() == "arm64" else "x86_64"
        asset = f"llvm-mingw-{version}-ucrt-ubuntu-22.04-{suffix}.tar.xz"
    elif any(fnmatch.fnmatchcase(sysname, glob) for glob in _MINGW_UNAME_GLOBS):
        asset = f"llvm-mingw-{version}-ucrt-x86_64.zip"
    else:
        # GitHub windows-latest Git Bash reports MINGW64_NT; Darwin is not used
        # for windows artifacts. cmd.exe still says OS=Windows_NT, hence the fallback.
        if "windows" in (os.environ.get("OS") or "").lower():
            asset = f"llvm-mingw-{version}-ucrt-x86_64.zip"
        else:
            log(f"unsupported host {sysname} for llvm-mingw")
            return FAIL_EXIT

    dest = prefix / asset
    marker = prefix / f"{asset}.ok"
    if not marker.is_file():
        log(f"downloading {base}/{asset}")
        if shutil.which("curl"):
            status = common.run([_tool("curl"), "-fsSL", "-o", str(dest), f"{base}/{asset}"])
        else:
            status = common.run([_tool("wget"), "-qO", str(dest), f"{base}/{asset}"])
        if status:
            return status
        if asset.endswith(".zip"):
            if shutil.which("unzip"):
                status = common.run([_tool("unzip"), "-q", "-o", str(dest), "-d", str(prefix)])
            else:
                # Git Bash / Windows without unzip: use tar if available (bsdtar).
                status = common.run([_tool("tar"), "-xf", str(dest), "-C", str(prefix)])
        else:
            status = common.run([_tool("tar"), "-xJf", str(dest), "-C", str(prefix)])
        if status:
            return status
        marker.touch()

    found = ""
    # sorted(): os.walk yields in directory-read order, which is not stable across
    # filesystems. One GITHUB_PATH line is a contract (C2), so pick the
    # alphabetically-first match like `find ... | sed -n 1p` did on an ext4 runner.
    for root, _dirs, files in sorted(os.walk(prefix)):
        for name in sorted(files):
            if name in _MINGW_GCC_NAMES and Path(root, name).is_file():
                found = str(Path(root))
                break
        if found:
            break
    if not found:
        log(f"llvm-mingw gcc not found under {prefix}")
        return FAIL_EXIT
    sys.stdout.write(f"{found}\n")
    return 0


# --------------------------------------------------------------------------- #
# frontend-build
# --------------------------------------------------------------------------- #

YARN_VERSION = "1.22.22"


def frontend_build(argv: list[str]) -> int:
    """`frontend-build` — yarn build so go:embed receives a real admin UI."""
    frontend = ROOT / "frontend"
    if not (frontend / "yarn.lock").is_file():
        log("missing frontend/yarn.lock")
        return FAIL_EXIT
    # Node 22 images / setup-node do not put Yarn on PATH until corepack is enabled.
    if shutil.which("corepack"):
        status = common.run([_tool("corepack"), "enable"], cwd=frontend)
        if status:
            return status
        status = common.run([_tool("corepack"), "prepare", f"yarn@{YARN_VERSION}", "--activate"], cwd=frontend)
        if status:
            return status
    if shutil.which("yarn") is None:
        log("yarn is required (enable corepack or install Yarn 1)")
        return FAIL_EXIT
    status = common.run([_tool("yarn"), "install", "--frozen-lockfile"], cwd=frontend)
    if status:
        return status
    # vite.config reads the very same KIRIVERS_{VERSION,BUILD_COMMIT,BUILD_TIME}
    # trio the Go build injects above, so one release stamps the frontend and the
    # binary from one source -- except that vite uses the injected commit verbatim
    # while `cmd/link` hands the trim to internal/buildinfo. Truncate here, so the
    # width rule stays in this file for both halves of an artifact.
    env = os.environ.copy()
    if env.get("KIRIVERS_BUILD_COMMIT"):
        env["KIRIVERS_BUILD_COMMIT"] = _short_commit(env["KIRIVERS_BUILD_COMMIT"])
    status = common.run([_tool("yarn"), "build"], cwd=frontend, env=env)
    if status:
        return status
    if not (frontend / "dist" / "index.html").is_file():
        log("yarn build did not produce frontend/dist/index.html")
        return FAIL_EXIT
    return 0
