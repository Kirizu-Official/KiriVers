"""Shared platform facts for the KiriVers maintainer CLI.

Contract ids (C1..C12) refer to `.trellis/tasks/09-20-build-python-cli/design.md`
section 4: the byte-level promises the GitHub workflows and release consumers
depend on. Renaming or reformatting anything marked with one of those ids is a
breaking change to the release plumbing.
"""

from __future__ import annotations

import hashlib
import os
import platform
import re
import shutil
import subprocess
import sys
from pathlib import Path

# This file lives at <root>/dev/build/kirivers_build/common.py. Resolved from the
# file location, never from $PWD: deriving the root from `pwd` is what made the
# old shell port silently drop the changelog type table on Windows (defect D7).
ROOT = Path(__file__).resolve().parents[3]

# One row per official target: goos goarch goarm libc os_token arch_tag.
# musl is limited to architectures a native runner can build (no QEMU anywhere).
MATRIX: tuple[tuple[str, str, str, str, str, str], ...] = (
    ("linux", "386", "-", "glibc", "Linux", "x86"),
    ("linux", "amd64", "-", "musl", "Linux", "x86_64"),
    ("linux", "arm", "7", "glibc", "Linux", "armv7"),
    ("linux", "arm64", "-", "musl", "Linux", "arm64"),
    ("linux", "riscv64", "-", "glibc", "Linux", "riscv64"),
    ("darwin", "amd64", "-", "none", "macOS", "x86_64"),
    ("darwin", "arm64", "-", "none", "macOS", "arm64"),
    ("windows", "386", "-", "none", "Windows", "x86"),
    ("windows", "amd64", "-", "none", "Windows", "x86_64"),
    ("windows", "arm64", "-", "none", "Windows", "arm64"),
)

# Expected `file(1)` machine signature per target; catches a cross compiler that
# silently fell back to the host architecture. Both spellings toolchains use for
# 32-bit x86 are accepted. Matched case-insensitively, like `grep -Eqi`.
MACHINE_PATTERNS: dict[str, str] = {
    "linux/386": r"ELF 32-bit.*(80386|i386)",
    "linux/amd64": r"ELF 64-bit.*x86-64",
    "linux/arm": r"ELF 32-bit.*ARM",
    "linux/arm64": r"ELF 64-bit.*aarch64",
    "linux/riscv64": r"ELF 64-bit.*RISC-V",
    "darwin/amd64": r"Mach-O 64-bit.*(x86_64|amd64)",
    "darwin/arm64": r"Mach-O 64-bit.*arm64",
    "windows/386": r"PE32 executable.*(80386|i386)",
    # `\+` is a literal plus; dropping the backslash would change the pattern.
    "windows/amd64": r"PE32\+ executable.*x86-64",
    "windows/arm64": r"PE32\+ executable.*(aarch64|arm64)",
}

USAGE_EXIT = 2
FAIL_EXIT = 1


def force_lf() -> None:
    """Pin stdout/stderr to LF.

    The shell originals always emitted LF; Python translates `print` to CRLF on
    Windows, which would corrupt everything a workflow captures: `eval` lines,
    the guard tokens, release notes and the PR comment body. Runners are Linux so
    only a maintainer's own box would show it, which is exactly why it is pinned
    here instead of left to the platform.
    """
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, "reconfigure"):
            # errors="replace" too: a report line that carries child output outside
            # the host console codepage (a Chinese fixture dump on a cp936 Windows
            # box) must not raise UnicodeEncodeError and lose the whole verdict.
            stream.reconfigure(newline="\n", errors="replace")


def write_lf(path: Path, text: str) -> None:
    """Write a file with LF endings regardless of host platform."""
    with path.open("w", encoding="utf-8", newline="\n") as handle:
        handle.write(text)


def log(message: str = "") -> None:
    """Diagnostics go to stderr so the stdout contracts stay byte-clean (C12)."""
    print(message, file=sys.stderr)


def matrix_row(goos: str, goarch: str) -> tuple[str, str, str, str, str, str]:
    for row in MATRIX:
        if row[0] == goos and row[1] == goarch:
            return row
    raise LookupError(f"unsupported {goos}/{goarch}")


def asset_name(goos: str, goarch: str) -> str:
    """Internal binary name inside every archive (stable across releases)."""
    base = f"kirivers-{goos}-{goarch}"
    return f"{base}.exe" if goos == "windows" else base


def archive_name(goos: str, goarch: str, sha6: str | None = None) -> str:
    """Release asset name: KiriVers-<OS>-<Arch>-<sha6>.zip.

    Without an explicit sha6 the literal `<sha6>` placeholder is kept (C4/C10):
    release.yml seds `-<sha6>.zip` into a glob to verify the uploaded assets.
    """
    row = matrix_row(goos, goarch)
    return f"KiriVers-{row[4]}-{row[5]}-{sha6 or '<sha6>'}.zip"


def host_goarch() -> str:
    """Map the host machine to a GOARCH, matching `uname -m` folding."""
    machine = platform.machine().lower()
    return {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(
        machine, machine
    )


def host_os() -> str:
    """uname -s equivalent, including the Git-Bash MINGW/MSYS spellings."""
    if os.name != "nt":
        return platform.system()
    # MSYS reports MINGW64_NT-10.0 via uname; Python only says "Windows".
    msys = os.environ.get("MSYSTEM") or os.environ.get("OSTYPE") or ""
    return f"{msys}_NT" if msys else "Windows"


def is_musl_host() -> bool:
    if Path("/etc/alpine-release").is_file():
        return True
    try:
        proc = subprocess.run(
            ["ldd", "/bin/sh"], capture_output=True, text=True, check=False
        )
    except OSError:
        return False
    return "musl" in proc.stdout + proc.stderr


def go_minor(root: Path = ROOT) -> str:
    """go.mod `go 1.27.0` -> `1.27` (the Docker Hub golang:<minor>-alpine tag)."""
    match = re.search(r"^go\s+(\d+)\.(\d+)", (root / "go.mod").read_text(encoding="utf-8"), re.M)
    if not match:
        raise LookupError("go.mod has no `go <version>` line")
    return f"{match.group(1)}.{match.group(2)}"


def golang_alpine_image(root: Path = ROOT) -> str:
    return f"golang:{go_minor(root)}-alpine"


def sha256_file(path: Path) -> str:
    """Hex digest of a file, streamed so no archive-sized buffer is allocated."""
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def assert_machine(goos: str, goarch: str, *binaries: Path) -> int:
    """Fail when a binary's machine type is not the requested target.

    Skips without failing on hosts without file(1) (Git Bash on Windows), which
    is the normal path for every windows/* release build.
    """
    if shutil.which("file") is None:
        for binary in binaries:
            log(f"assert-machine: file(1) missing, skipping {binary}")
        return 0
    try:
        pattern = re.compile(MACHINE_PATTERNS[f"{goos}/{goarch}"], re.IGNORECASE)
    except KeyError:
        log(f"assert-machine: unsupported {goos}/{goarch}")
        return USAGE_EXIT
    for binary in binaries:
        if not binary.is_file():
            log(f"assert-machine: missing {binary}")
            return FAIL_EXIT
        proc = subprocess.run(
            ["file", "-b", str(binary)], capture_output=True, text=True, check=False
        )
        info = proc.stdout.strip() or proc.stderr.strip()
        if not pattern.search(info):
            log(
                f"assert-machine: {binary} is not {goos}/{goarch} "
                f"(expected /{MACHINE_PATTERNS[f'{goos}/{goarch}']}/, got:{info})"
            )
            return FAIL_EXIT
        log(f"ok machine {goos}/{goarch}: {binary}")
    return 0


def has_label(labels_csv: str, name: str) -> bool:
    """Match one label in a comma-separated list, trimming padding.

    One implementation for both guards: the shell port had a trimming version in
    pr-guard.sh and a non-trimming one in check-issue-link.sh, so the same record
    could classify two ways (defect D3).
    """
    return any(candidate == name for candidate in (part.strip() for part in labels_csv.split(",")))


def record_field(key: str, record: Path) -> str:
    """Read `key=value` from a guard record file; first match wins.

    Mirrors the old awk helper: the key must start the line and the value is
    everything after the first `=`, unquoted.
    """
    try:
        lines = record.read_text(encoding="utf-8").splitlines()
    except OSError:
        return ""
    prefix = f"{key}="
    for line in lines:
        if line.startswith(prefix):
            return line[len(prefix) :]
    return ""


def run(command: list[str], cwd: Path | None = None, env: dict[str, str] | None = None) -> int:
    """Run a child, sending its stdout to our stderr.

    Every contract line this CLI emits lives on stdout; a child that writes
    progress there would land inside an `eval "$(...)"` in the workflow (defect
    D1), so child noise is never allowed through.
    """
    proc = subprocess.run(
        command, cwd=str(cwd) if cwd else None, env=env, stdout=sys.stderr, check=False
    )
    return proc.returncode
