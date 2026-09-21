"""Dispatcher for the maintainer CLI.

Subcommands parse their own arguments instead of sharing one argparse tree: the
shell originals have per-command quirks the workflows depend on (for example
`next-version` honours `--format` only as the first argument and silently treats
anything else as plain mode), and an argparse layer would smooth those out
exactly where smoothing is a contract break.
"""

from __future__ import annotations

import importlib
import sys
from typing import Callable

from . import common

# name -> (module, entrypoint, one-line usage for `--help`)
COMMANDS: dict[str, tuple[str, str, str]] = {
    "build-cgo": ("build", "build_cgo", "<goos> <goarch> <outfile>"),
    "build-linux-musl": ("build", "build_linux_musl", "<amd64|arm64> [outfile]"),
    "assert-linux-musl": ("build", "assert_linux_musl", "<binary>..."),
    "install-cross-toolchains": ("build", "install_cross_toolchains", "<386|arm|riscv64>"),
    "install-llvm-mingw": ("build", "install_llvm_mingw", ""),
    "frontend-build": ("build", "frontend_build", ""),
    "next-version": ("version", "next_version", "[--format <plain|env>]"),
    "release-notes": ("notes", "release_notes", "[options] [prev_tag] [head]"),
    "changelog": ("notes", "changelog", "[release-notes options]"),
    "asset-names": ("package", "asset_names", "[--binaries] [linux|darwin|windows]"),
    "package-assets": ("package", "package_assets", "<srcdir> <destdir>"),
    "docker-image": ("images", "docker_image", "<amd64|arm64> <semver>"),
    "docker-manifest": ("images", "docker_manifest", "<semver> [arch...]"),
    "commitlint": ("guards", "commitlint", "(driven by COMMITLINT_TITLE/FROM/TO)"),
    "issue-link": ("guards", "issue_link", "[--record <file>] [pr_number]"),
    "guard": ("guards", "pr_guard", "--record <file> [--title-ok] [--issue-ok] [--out <file>]"),
    "check-workflows": ("selfcheck", "check_workflows", ""),
}


def usage() -> str:
    width = max(len(name) for name in COMMANDS)
    lines = ["usage: kirivers.py <command> [args]", "", "commands:"]
    lines += [f"  {name.ljust(width)}  {sig}" for name, (_, _, sig) in sorted(COMMANDS.items())]
    lines.append("\nEvery command prints diagnostics on stderr; stdout carries only")
    lines.append("what a workflow captures (eval lines, tokens, note bodies).")
    return "\n".join(lines)


def main(argv: list[str]) -> int:
    common.force_lf()
    if not argv or argv[0] in {"-h", "--help", "help"}:
        print(usage())
        return 0
    name, *rest = argv
    entry = COMMANDS.get(name)
    if entry is None:
        print(f"kirivers: unknown command {name}", file=sys.stderr)
        print(usage(), file=sys.stderr)
        return 2
    module_name, function_name, _ = entry
    module = importlib.import_module(f"{__package__}.{module_name}")
    run: Callable[[list[str]], int] = getattr(module, function_name)
    return run(rest)
