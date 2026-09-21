"""Static regression gate for the release plumbing.

Ported from `dev/build/check-workflows.sh`. Same idea as the original: no
remote, no registry login, no actionlint — just assertions over tracked files
plus the offline self-check suites, so a maintainer can run it before pushing.

The report goes to stdout (nothing parses it; it is for humans and for CI logs)
and the verdict to stderr.
"""

from __future__ import annotations

import ast
import contextlib
import io
import re
import subprocess
import sys
from pathlib import Path

from . import common
from .cli import COMMANDS

REL = ".github/workflows/release.yml"
CI = ".github/workflows/ci.yml"
GUARD = ".github/workflows/pr-guard.yml"
SEC = ".github/workflows/security-gate.yml"
SCAN = ".github/workflows/gosec-scan.yml"
SDKAM = ".github/workflows/sdk-automerge.yml"

# Oldest interpreter the CLI must run on: macOS runners ship a python3 3.9.x that
# `brew install` cannot override, so 3.10+ syntax has to be rejected statically.
_MIN_PYTHON = (3, 9)

# Files the CLI is allowed to be made of; anything else under dev/build is drift.
CLI_FILES = (
    "dev/build/kirivers.py",
    "dev/build/kirivers_build/__init__.py",
    "dev/build/kirivers_build/cli.py",
    "dev/build/kirivers_build/common.py",
    "dev/build/kirivers_build/build.py",
    "dev/build/kirivers_build/version.py",
    "dev/build/kirivers_build/notes.py",
    "dev/build/kirivers_build/package.py",
    "dev/build/kirivers_build/images.py",
    "dev/build/kirivers_build/guards.py",
    "dev/build/kirivers_build/selfcheck.py",
)

# This module names the retired pipeline files on purpose, so the GitLab sweep
# has to skip it — the same exemption the shell version gave itself.
GITLAB_SWEEP_SKIP = {"dev/build/kirivers_build/selfcheck.py"}


class Checker:
    def __init__(self) -> None:
        self.lines: list[str] = []
        self.failed = False

    def ok(self, message: str) -> None:
        self.lines.append(f"ok   {message}")

    def fail(self, message: str) -> None:
        self.lines.append(f"FAIL {message}")
        self.failed = True

    def note(self, message: str) -> None:
        self.lines.append(message)

    def assert_file(self, path: str) -> None:
        if (common.ROOT / path).is_file():
            self.ok(f"file exists: {path}")
        else:
            self.fail(f"missing file: {path}")

    def refute_file(self, path: str) -> None:
        if (common.ROOT / path).exists():
            self.fail(f"file must not exist: {path}")
        else:
            self.ok(f"absent: {path}")

    def _text(self, path: str) -> str | None:
        target = common.ROOT / path
        if not target.is_file():
            self.fail(f"{path} is not readable")
            return None
        return target.read_text(encoding="utf-8", errors="replace")

    def assert_grep(self, path: str, pattern: str, why: str) -> None:
        text = self._text(path)
        if text is None:
            return
        if re.search(pattern, text, re.MULTILINE):
            self.ok(f"{path}: {why}")
        else:
            self.fail(f"{path} lacks /{pattern}/ ({why})")

    def refute_grep(self, path: str, pattern: str, why: str) -> None:
        text = self._text(path)
        if text is None:
            return
        if re.search(pattern, text, re.MULTILINE):
            self.fail(f"{path} must not contain /{pattern}/ ({why})")
        else:
            self.ok(f"{path}: {why}")


def _check_publish_is_manual(checker: Checker) -> None:
    # --- server publish is manual-only ---------------------------------------
    checker.assert_file(REL)
    checker.assert_grep(REL, r"^on:", "has a trigger block")
    checker.assert_grep(REL, r"workflow_dispatch", "triggered by workflow_dispatch")
    text = checker._text(REL) or ""
    trigger_block = text.split("jobs:")[0] if "jobs:" in text else text
    if re.search(r"^[ \t]*push:", trigger_block, re.MULTILINE):
        checker.fail(f"{REL} must not contain /^[ \\t]*push:/ (never runs on a push to main)")
    else:
        checker.ok(f"{REL}: never runs on a push to main")
    if re.search(r"^[ \t]*tags?:", trigger_block, re.MULTILINE):
        checker.fail(f"{REL} must not contain /^[ \\t]*tags?:/ (never runs on a tag)")
    else:
        checker.ok(f"{REL}: never runs on a tag")
    if "pull_request" in trigger_block:
        checker.fail(f"{REL}: not tied to pull requests")
    else:
        checker.ok(f"{REL}: not tied to pull requests")


def _check_no_emulation(checker: Checker) -> None:
    # --- no emulation anywhere ----------------------------------------------
    for path, why in (
        (REL, "no QEMU setup"),
        (CI, "CI has no QEMU setup"),
        (GUARD, "guard has no QEMU setup"),
    ):
        checker.refute_grep(path, r"setup-qemu-action", why)
    checker.refute_grep(REL, r"binfmt", "no binfmt registration")


def _check_matrix_and_wiring(checker: Checker) -> None:
    # --- matrix: per-arch native runners ------------------------------------
    checker.assert_grep(REL, r"ubuntu-24\.04-arm", "arm64 runs on a native arm64 runner")
    checker.assert_grep(REL, r"macos-15", "darwin builds on macOS")
    checker.assert_grep(
        REL, r"goarch: \[386, arm, riscv64\]", "glibc cross builds stay on amd64"
    )
    checker.assert_grep(
        REL, r"goarch: \[386, amd64, arm64\]", "windows covers three architectures"
    )

    # --- the workflows must call the CLI, and call commands that exist ------
    for path, pattern, why in (
        (REL, r"kirivers\.py package-assets", "archives are built from the raw binaries"),
        (REL, r"kirivers\.py build-linux-musl", "linux musl path is used"),
        (REL, r"kirivers\.py build-cgo", "cross builds go through build-cgo"),
        (REL, r"kirivers\.py release-notes", "release notes come from the CLI"),
        (REL, r"kirivers\.py changelog", "CHANGELOG.md is written back to main"),
        (REL, r"softprops/action-gh-release", "GitHub Release is created via official action"),
        (REL, r"docker/build-push-action", "per-architecture images are pushed via build-push-action"),
        (REL, r"docker buildx imagetools create", "multi-arch manifest is composed remotely"),
        (REL, r"kirivers\.py next-version", "the version is derived, not typed"),
        (CI, r"kirivers\.py frontend-build", "CI builds the admin UI"),
        (CI, r"kirivers\.py issue-link", "CI gates the issue link"),
        (CI, r"kirivers\.py check-workflows", "CI runs this gate"),
        (CI, r"kirivers\.py commitlint", "CI lints through the CLI"),
        (CI, r"kirivers\.py build-cgo", "CI compiles with the release builder"),
        (REL, r"kirivers\.py install-cross-toolchains", "cross CC/CXX come from the CLI"),
        (REL, r"kirivers\.py install-llvm-mingw", "the windows toolchain comes from the CLI"),
        (REL, r"kirivers\.py asset-names", "uploaded assets are checked against the CLI list"),
        (GUARD, r"kirivers\.py commitlint", "the guard lints the title through the CLI"),
        (GUARD, r"kirivers\.py issue-link", "the guard checks the issue link through the CLI"),
        (GUARD, r"kirivers\.py guard", "the verdict comes from the CLI"),
    ):
        checker.assert_grep(path, pattern, why)

    # The windows job cannot see `python3`: the image ships `python`.
    checker.assert_grep(
        REL, r"python dev/build/kirivers\.py", "the windows job calls the CLI through python"
    )
    checker.refute_grep(
        REL,
        r"python3 dev/build/kirivers\.py build-cgo windows",
        "the windows job never uses python3",
    )
    # The toolchain step must not be able to fail silently into an empty PATH line.
    checker.refute_grep(
        REL,
        r'echo "\$\(python dev/build/kirivers\.py install-llvm-mingw\)"',
        "a failed llvm-mingw install must abort the job",
    )

    # A toolchain cache must miss when the pinned version changes, and must never
    # fall back to an older entry: `restore-keys` would do exactly that.
    checker.assert_grep(
        REL,
        r"key: llvm-mingw-\$\{\{ hashFiles\('dev/build/kirivers_build/build\.py'\)",
        "the llvm-mingw cache key follows the pinned toolchain version",
    )
    # Match the YAML key, not the phrase: a comment explaining why the fallback is
    # omitted must not trip the assertion that keeps it omitted (the bare-substring
    # refutes in the old shell gate had exactly this flaw).
    checker.refute_grep(REL, r"^[ \t]*restore-keys:", "no cache may fall back to an older toolchain")
    checker.assert_grep(REL, r"cache: yarn", "the release frontend job caches yarn downloads")
    checker.assert_grep(CI, r"cache: yarn", "CI caches yarn downloads")
    checker.assert_grep(CI, r"cache-dependency-path: frontend/yarn\.lock", "the yarn key is the lockfile")

    # A retired verb must not come back as a subcommand either.
    checker.refute_grep(REL, r"docker-push", "the old single-command push path is gone")


def _check_asset_name_contract(checker: Checker) -> None:
    from . import package

    archives = [
        line
        for line in _captured(package.asset_names, [])[0].splitlines()
        if line.startswith("KiriVers-")
    ]
    if len(archives) != 10:
        checker.fail(f"asset-names printed {len(archives)} archives, expected 10")
    else:
        checker.ok("asset-names prints the 10 official archives")
    names = _captured(package.asset_names, [])[0].splitlines()
    if "frontend-dist.zip" in names:
        checker.ok("asset-names lists frontend-dist.zip")
    else:
        checker.fail("asset-names must list frontend-dist.zip")
    if "<sha6>" in "".join(names):
        checker.ok("asset-names keeps the literal <sha6> placeholder release.yml globs on")
    else:
        checker.fail("asset-names lost the <sha6> placeholder that release.yml expands")
    binaries = _captured(package.asset_names, ["--binaries"])[0].splitlines()
    if "kirivers-windows-386.exe" in binaries:
        checker.ok("asset-names --binaries includes windows/386")
    else:
        checker.fail("asset-names --binaries must include windows/386")


def _captured(function, argv: list[str]) -> tuple[str, int]:
    """Run a subcommand with stdout captured; stderr stays on the console."""
    buffer = io.StringIO()
    with contextlib.redirect_stdout(buffer):
        status = function(list(argv))
    return buffer.getvalue(), status


def _check_layout_invariants(checker: Checker) -> None:
    # --- dev/build holds the CLI, not a compose stack ------------------------
    for path in ("dev/build/changelog-types.conf",) + CLI_FILES:
        checker.assert_file(path)
    strays = sorted(
        str(path.relative_to(common.ROOT))
        for path in (common.ROOT / "dev/build").rglob("*")
        if path.is_file()
        and path.suffix in {".sh", ".yml"}
        and "__pycache__" not in path.parts
    )
    if strays:
        checker.fail(f"dev/build must hold no shell or compose file, found {', '.join(strays)}")
    else:
        checker.ok("dev/build holds no shell script and no compose file")
    for path in (
        "dev/build/compose.yml",
        "dev/build/.env.example",
        "dev/build/seed-config.sh",
        "dev/build/build-darwin.sh",
        "dev/build/build-windows.sh",
        "dev/build/ensure-go.sh",
    ):
        checker.refute_file(path)
    checker.assert_file("deploy/compose.yml")
    checker.assert_file("dev/docker/compose.yml")


def _check_retired_forge(checker: Checker) -> None:
    # --- retired GitLab path -------------------------------------------------
    for path in (
        ".gitlab-ci.yml",
        "dev/build/gitlab-release.sh",
        "dev/build/docker-push.sh",
        "dev/build/checksums.sh",
    ):
        checker.refute_file(path)
    # `**/` also matches zero directories, so this sweeps dev/build/kirivers.py
    # *and* the package subdirectory: the shell sweep covered every helper that
    # held release logic, and shrinking it to the top level would silently drop
    # the ten modules where that logic lives now.
    # as_posix(): GITLAB_SWEEP_SKIP is spelled with forward slashes, and on a
    # Windows maintainer host Path-relative yields backslashes, which would make
    # the exemption never match and the gate flag its own assertion strings.
    swept = sorted(
        path.relative_to(common.ROOT).as_posix()
        for pattern in (".github/workflows/*.yml", "dev/build/**/*.py")
        for path in common.ROOT.glob(pattern)
        if path.relative_to(common.ROOT).as_posix() not in GITLAB_SWEEP_SKIP
    )
    offenders = [rel for rel in swept if "gitlab" in (common.ROOT / rel).read_text(encoding="utf-8").lower()]
    if offenders:
        for rel in offenders:
            checker.fail(f"{rel} still mentions GitLab")
    else:
        checker.ok(f"no workflow or build file mentions GitLab ({len(swept)} swept)")


def _check_ci_does_not_publish(checker: Checker) -> None:
    checker.assert_file(CI)
    checker.assert_grep(CI, r"go build", "CI compiles the binary")
    checker.assert_grep(CI, r"go test", "CI runs the test suite")
    checker.refute_grep(CI, r"gh release", "CI never creates a release")
    checker.refute_grep(CI, r"docker push|buildx build", "CI never pushes an image")
    checker.refute_grep(CI, r"git tag", "CI never tags a release")
    checker.assert_grep(CI, r"github\.base_ref == 'main'", "the issue gate is main-only")
    checker.assert_grep(CI, r"^[ \t]*push:", "CI also runs on pushes to main")


def _check_guard_invariants(checker: Checker) -> None:
    # --- pr-guard invariants -------------------------------------------------
    checker.assert_file(GUARD)
    checker.assert_grep(GUARD, r"^[ \t]*pull_request_target:", "guard uses pull_request_target")
    checker.refute_grep(GUARD, r"^[ \t]*pull_request:", "guard has no plain pull_request trigger")
    checker.refute_grep(GUARD, r"refs/pull", "guard never fetches a pull request ref")
    checker.refute_grep(
        GUARD, r"head\.sha|head\.ref|head_repo", "guard never reads the pull request head"
    )
    checker.refute_grep(
        GUARD,
        r"go build|go test|\byarn\b|\bnpm\b|\bsetup-go\b|\bsetup-python\b|\bpip\b"
        r"|\brequirements\.txt\b|\bpnpm\b|\bdeno\b|\bbun\b|\bactions/cache\b",
        "guard executes no project code and installs no dependency",
    )
    checker.refute_grep(GUARD, r"docker", "guard runs no container build")
    checker.assert_grep(GUARD, r"ref: main", "guard checks out the base branch only")
    checker.assert_grep(GUARD, r"base\.ref == 'main'", "guard is main-only")
    checker.assert_grep(GUARD, r"contents: read", "guard permissions: contents read")
    checker.assert_grep(GUARD, r"issues: read", "guard permissions: issues read")
    checker.assert_grep(GUARD, r"pull-requests: write", "guard permissions: pull-requests write")
    # D5: a broken check must degrade to `error`, never to a closed pull request.
    checker.assert_grep(GUARD, r"ok=error", "guard maps a failed check to the error state")


def _check_security_gate_invariants(checker: Checker) -> None:
    # --- security-gate.yml: label state machine, zero code execution ---------
    # The gate holds pull-requests:write on a pull_request_target token, so it
    # must never check out or run anything (design.md §3.2); the scan over
    # untrusted code lives in the secretless gosec-scan.yml instead.
    for path in (SEC, SCAN):
        checker.assert_file(path)
    checker.assert_grep(SEC, r"^[ \t]*pull_request_target:", "gate uses pull_request_target")
    checker.assert_grep(SEC, r"synchronize", "gate revokes labels on new commits")
    checker.assert_grep(SEC, r'ready-merge,wait-merge', "gate revokes both merge labels")
    checker.assert_grep(SEC, r"workflow_run:", "gate promotes via workflow_run")
    checker.refute_grep(SEC, r"actions/checkout", "gate checks out no code at all")
    checker.refute_grep(SEC, r"pull_request\.head", "gate never reads the pull request head")
    # The scan workflow is the one that touches PR code, so it must be the
    # zero-secret context: plain pull_request, read-only, never the target.
    checker.assert_grep(SCAN, r"^[ \t]*pull_request:$", "scan runs in the untrusted pull_request context")
    checker.assert_grep(SCAN, r"securego/gosec", "scan runs the gosec analyzer")
    checker.assert_grep(SCAN, r"wait-merge", "scan triggers on the wait-merge label")
    checker.refute_grep(SCAN, r"pull_request_target", "scan never uses pull_request_target")
    checker.refute_grep(SCAN, r"contents: write|pull-requests: write", "scan holds no write permission")
    # sdk-automerge must not ask for permissions its steps never use, and its
    # registry push must survive a missing token as a warning, not a green lie.
    checker.assert_grep(SDKAM, r"workflow_dispatch", "sdk publish is dispatch-only")
    checker.refute_grep(SDKAM, r"id-token:", "sdk publish requests no OIDC it never uses")
    checker.assert_grep(SDKAM, r"::warning::Missing secret", "missing registry token warns and skips")
    checker.assert_grep(SDKAM, r"katyo/publish-crates@v2", "rust publishes via the design.md action")
    checker.assert_grep(SDKAM, r"pypa/gh-action-pypi-publish@release/v1", "python publishes via the design.md action")


def _check_offline_suites(checker: Checker) -> None:
    # The suites are run as separate processes, exactly like the shell gate did:
    # they build throwaway git repos and need real file descriptors, which an
    # in-process stdout redirect would not give them.
    entry = common.ROOT / "dev" / "build" / "kirivers.py"
    for label, argv in (
        ("guard --self-check", ["guard", "--self-check"]),
        ("issue-link --self-check", ["issue-link", "--self-check"]),
        ("release-notes --self-check", ["release-notes", "--self-check"]),
    ):
        # encoding is not optional: the suites print Chinese and emoji on stderr,
        # and text=True alone decodes them with the host locale (cp936 on a Windows
        # maintainer box), which kills the reader thread and turns a real FAIL into
        # `None + None`.
        proc = subprocess.run(
            [sys.executable, str(entry), *argv],
            cwd=common.ROOT,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            check=False,
        )
        if proc.returncode == 0:
            checker.ok(label)
        else:
            checker.fail(label)
            checker.note(
                "".join(part for part in (proc.stdout, proc.stderr) if part).rstrip("\n")
            )


def _check_python_sources_parse(checker: Checker) -> None:
    # The oldest plausible runner interpreter is macOS' python3 (3.9.x), so the
    # ceiling is pinned here instead of trusting whichever interpreter runs this.
    # ast.parse(feature_version=...) only rejects newer *syntax*; the stdlib API
    # surface (no tomllib / StrEnum / match / X | Y outside an annotation) is
    # reviewed in code review and by the same three self-check suites below.
    broken = 0
    for path in sorted((common.ROOT / "dev/build").rglob("*.py")):
        text = path.read_text(encoding="utf-8")
        try:
            compile(text, str(path), "exec")
            ast.parse(text, str(path), "exec", feature_version=_MIN_PYTHON)
        except SyntaxError as error:
            checker.fail(f"syntax: {path} {error}")
            broken += 1
    if not broken:
        checker.ok(f"every dev/build python file parses (>= {_MIN_PYTHON[0]}.{_MIN_PYTHON[1]})")


def _check_subcommands_exist(checker: Checker) -> None:
    called: set[str] = set()
    for path in (REL, CI, GUARD):
        text = (common.ROOT / path).read_text(encoding="utf-8")
        called |= set(re.findall(r"kirivers\.py ([a-z][a-z0-9-]*)", text))
    unknown = sorted(called - set(COMMANDS))
    if unknown:
        checker.fail(f"workflows call unregistered subcommands: {', '.join(unknown)}")
    else:
        checker.ok(f"all {len(called)} subcommands the workflows call are registered")


def _git_show(revision: str, path: str) -> str | None:
    proc = subprocess.run(
        ["git", "show", f"{revision}:{path}"],
        cwd=common.ROOT,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        check=False,
    )
    return proc.stdout if proc.returncode == 0 else None


def _check_sdk_branches(checker: Checker) -> None:
    # --- sdk/* branches (only when their refs are present) -------------------
    proc = subprocess.run(
        ["git", "for-each-ref", "--format=%(refname)", "refs/heads/sdk/*"],
        cwd=common.ROOT,
        capture_output=True,
        text=True,
        check=False,
    )
    refs = [line for line in proc.stdout.splitlines() if line.strip()]
    seen = 0
    for ref in refs:
        branch = ref[len("refs/heads/") :]
        seen += 1
        release = _git_show(ref, ".github/workflows/release.yml")
        if release is not None:
            if "types: [closed]" not in release:
                checker.fail(f"{branch} release.yml must trigger on pull_request closed")
            if "merged == true" not in release:
                checker.fail(f"{branch} release.yml must require a merged pull request")
            if re.search(r"^[ \t]*push:", release, re.MULTILINE):
                checker.fail(f"{branch} release.yml must not trigger on push")
            if "gitlab" in release.lower():
                checker.fail(f"{branch} release.yml still mentions GitLab")
        pipeline = _git_show(ref, ".github/workflows/ci.yml")
        if pipeline is not None and "issue-link" in pipeline:
            checker.fail(f"{branch} ci.yml must not carry the main-only issue gate")
        if _git_show(ref, ".gitlab-ci.yml") is not None:
            checker.fail(f"{branch} still has .gitlab-ci.yml")
    if seen == 0:
        checker.note("note sdk/* branch assertions skipped (no sdk/* refs in this clone)")
    else:
        checker.ok(f"checked {seen} sdk/* branch workflows")


def check_workflows(argv: list[str]) -> int:
    common.force_lf()
    checker = Checker()
    for step in (
        _check_publish_is_manual,
        _check_no_emulation,
        _check_matrix_and_wiring,
        _check_asset_name_contract,
        _check_layout_invariants,
        _check_retired_forge,
        _check_ci_does_not_publish,
        _check_guard_invariants,
        _check_security_gate_invariants,
        _check_offline_suites,
        _check_python_sources_parse,
        _check_subcommands_exist,
    ):
        step(checker)
    _check_sdk_branches(checker)

    print("\n".join(checker.lines))
    if checker.failed:
        common.log("check-workflows: FAILED")
        return common.FAIL_EXIT
    common.log("check-workflows: all assertions passed")
    return 0
