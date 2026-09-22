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

from . import build, common
from .cli import COMMANDS

REL = ".github/workflows/release.yml"
CI = ".github/workflows/ci.yml"
GUARD = ".github/workflows/pr-guard.yml"
SEC = ".github/workflows/security-gate.yml"
SCAN = ".github/workflows/gosec-scan.yml"
SDKAM = ".github/workflows/sdk-automerge.yml"
BUILD = "dev/build/kirivers_build/build.py"

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


# The names the Actions expression evaluator accepts. This is a deliberate union,
# not a transcription of one doc page: the reference table lists `contains`,
# `startsWith`, `endsWith`, `format`, `join`, `toJSON`, `fromJSON`, `hashFiles`,
# `case` and the four job status functions, while `toBool`, `toInt`, `toDouble`,
# `toString`, `hashCode`, `equals`, `min` and `max` are accepted by the evaluator
# without appearing in that table. The question this gate asks is "will GitHub
# parse the file at all", so the wider set is the right one: an unsupported name is
# not a runtime error, GitHub rejects the entire file and no job in it can start --
# which is exactly how `substring(github.sha, 0, 7)` made release.yml unrunnable
# while every other assertion stayed green. If one of the undocumented names is ever
# refused in practice, delete it here: the failure mode is a local FAIL naming the
# file and line, not a broken release. Names are case-insensitive in Actions, so
# this set is lowercase and lookups fold.
ACTIONS_FUNCTIONS = frozenset(
    {
        "contains",
        "startswith",
        "endswith",
        "format",
        "join",
        "tojson",
        "fromjson",
        "case",
        "hashfiles",
        "tobool",
        "toint",
        "todouble",
        "tostring",
        "hashcode",
        "equals",
        "min",
        "max",
        "always",
        "success",
        "failure",
        "cancelled",
    }
)
_EXPRESSION_RE = re.compile(r"\{\{(.*?)\}\}", re.DOTALL)
_CALL_RE = re.compile(r"([A-Za-z_][A-Za-z0-9_]*)[ \t]*\(")
_STRING_RE = re.compile(r"'[^']*'")
# An `if:` key is an expression without anybody writing ${{ }} -- GitHub evaluates
# `if: startsWith(github.base_ref, 'sdk/')` and the multi-line `if: |` block the
# same way it evaluates a braced one, so an unsupported function hidden there
# breaks the file exactly as `substring()` did. A shell `if` inside a run block is
# `if …; then`, never `if:`, so this cannot read script text as an expression.
_IF_RE = re.compile(r"[ \t]*(?:-[ \t]+)?if:[ \t]*(.*)$")
_BLOCK_SCALAR_RE = re.compile(r"[>|][-+0-9]*[ \t]*(?:#.*)?$")


def _expression_payloads(text: str) -> list[tuple[str, int]]:
    """Every expression payload in one workflow file, with its starting line.

    Two shapes: an explicit ``${{ … }}`` anywhere in the file, and the value of an
    ``if:`` key (inline or block scalar). Quoted spans are stripped by the caller,
    because a string literal is data -- ``fromJSON('[\"OWNER\"]')`` must not be read
    as a call to a function named ``OWNER``.
    """
    found = [
        (match.group(1), text.count("\n", 0, match.start()) + 1)
        for match in _EXPRESSION_RE.finditer(text)
    ]
    lines = text.splitlines()
    for index, line in enumerate(lines):
        key = _IF_RE.match(line)
        if key is None:
            continue
        rest = key.group(1).strip()
        if _BLOCK_SCALAR_RE.match(rest):
            indent = len(line) - len(line.lstrip())
            body: list[str] = []
            for follow in lines[index + 1 :]:
                if follow.strip() and len(follow) - len(follow.lstrip()) <= indent:
                    break
                body.append(follow.strip())
            found.append((" ".join(body), index + 1))
        elif rest and not rest.startswith("#"):
            found.append((rest, index + 1))
    return found


def _check_workflow_expressions(checker: Checker) -> None:
    # --- every expression must use functions GitHub actually provides ---------
    checked = 0
    rejected = 0
    # .yaml is a supported extension too; skipping it would leave a whole workflow
    # file unparsed by the one gate whose job is to catch an unparsable file.
    workflows = sorted(
        path
        for pattern in (".github/workflows/*.yml", ".github/workflows/*.yaml")
        for path in common.ROOT.glob(pattern)
    )
    for path in workflows:
        text = path.read_text(encoding="utf-8", errors="replace")
        checked += 1
        for payload, line in _expression_payloads(text):
            # Quoted text is data, not code: `format('{0}', …)` must not be read as
            # a call to a function named `{0}`.
            stripped = _STRING_RE.sub("", payload)
            for call in _CALL_RE.finditer(stripped):
                name = call.group(1)
                if name.lower() in ACTIONS_FUNCTIONS:
                    continue
                rejected += 1
                checker.fail(
                    f"{path.relative_to(common.ROOT).as_posix()}:{line}: "
                    f"unrecognized function '{name}' inside the expression "
                    f"{payload.strip()[:60]!r}; GitHub refuses to parse the whole workflow file"
                )
    # Not a summary line about how many files were clean: the pass claim has to
    # disappear with the failures, or the log reads "6 files are fine" three lines
    # below the file it just refused.
    if checked and not rejected:
        checker.ok(f"{checked} workflow files use only supported expression functions")


def _check_build_info_stamp(checker: Checker) -> None:
    # --- one place decides the commit width, and it is not the workflow -------
    checker.assert_grep(
        REL,
        r"KIRIVERS_BUILD_COMMIT: \$\{\{ github\.sha \}\}",
        "injects the full SHA (there is no string-slicing function in an expression)",
    )
    checker.assert_grep(
        REL, r"KIRIVERS_BUILD_TIME: \$\{\{ github\.run_started_at \}\}", "one build time per run"
    )
    checker.assert_grep(BUILD, r"^_COMMIT_SHORT_LEN = 7", "the CLI owns the 7-character width")
    checker.assert_grep(BUILD, r"value = _short_commit\(value\)", "the injected SHA is trimmed")
    checker.assert_grep(
        BUILD,
        r"env\[.KIRIVERS_BUILD_COMMIT.\] = _short_commit",
        "the frontend build is trimmed to the same width",
    )
    # Behaviour, not spelling: internal/buildinfo truncates a VCS stamp to 7, so an
    # injected 40-char SHA and a local `git rev-parse --short` must agree.
    for raw, expected in (("a" * 40, "a" * 7), ("abcdef1", "abcdef1"), ("", "")):
        got = build._short_commit(raw)
        if got != expected:
            checker.fail(f"_short_commit({len(raw)} chars) returned {got!r}, expected {expected!r}")
    if build._COMMIT_SHORT_LEN != 7:
        checker.fail("_COMMIT_SHORT_LEN must stay 7 to match internal/buildinfo")
    else:
        checker.ok("_short_commit trims to internal/buildinfo's 7-character width")


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


def _check_cgo_include_roots(checker: Checker) -> None:
    # --- no include root may hold a dot-less file ------------------------------
    # A `-I` directory is searched case-insensitively on Windows and macOS, so a file
    # named like a standard header answers `#include <version>` before the toolchain
    # does: `third_party/hdiffpatch/VERSION` (the pinned upstream tag) broke every
    # libc++ CGO build there while Linux passed on a case-sensitive fs with
    # libstdc++. Standard header names never carry a dot, so a dot-less file in an
    # include root is a whole-matrix hazard — the vendored tree is include-relative.
    roots = {}
    proc = subprocess.run(
        ["git", "ls-files", "-z", "*.go"],
        cwd=common.ROOT,
        capture_output=True,
        text=True,
        check=False,
    )
    for rel in sorted(filter(None, proc.stdout.split("\0"))):
        text = (common.ROOT / rel).read_text(encoding="utf-8", errors="replace")
        for line in text.splitlines():
            if not line.lstrip().startswith("#cgo") or "FLAGS" not in line:
                continue
            for raw in re.findall(r"-I(\S+)", line):
                expanded = raw.replace("${SRCDIR}", str(common.ROOT / rel / ".."))
                roots.setdefault(Path(expanded).resolve().as_posix(), rel)

    hazards = []
    base = common.ROOT.resolve().as_posix()
    for root, declared_by in sorted(roots.items()):
        directory = Path(root)
        if not directory.is_dir():
            continue
        shown = root[len(base) + 1 :] if root.lower().startswith(f"{base.lower()}/") else root
        hazards += [
            f"{shown}/{entry.name} (-I in {declared_by})"
            for entry in sorted(directory.iterdir())
            # C headers all end in .h, so only the dot-less C++ standard names can be
            # shadowed. The legal-attribution names a vendored tree must ship are
            # dot-less too but are not header names, so they are excused by spelling.
            if entry.is_file()
            and "." not in entry.name
            and entry.name.upper() not in {"LICENSE", "COPYING", "NOTICE"}
        ]
    if hazards:
        for hazard in hazards:
            checker.fail(f"cgo include root can shadow a standard header: {hazard}")
    elif roots:
        checker.ok(f"{len(roots)} cgo include root(s) hold no dot-less file")
    else:
        checker.ok("no #cgo -I include root anywhere (vendored headers resolve relatively)")


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
        # The commit gate's own suite must stay offline: it asserts the tool
        # directory layout and the fault exit code without ever running npm.
        ("commitlint --self-check", ["commitlint", "--self-check"]),
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
    # --- sdk/* branches -------------------------------------------------------
    # Remote-tracking refs are the ones a CI checkout has: `actions/checkout` fills
    # refs/remotes/origin, never refs/heads, so scanning only refs/heads (as this
    # function did) made every assertion below a no-op in CI and let a branch ship
    # with a workflow that cannot trigger. Local refs are listed first so a
    # maintainer validates their own in-progress branch rather than the pushed one.
    refs: dict[str, str] = {}
    for pattern in ("refs/heads/sdk/*", "refs/remotes/origin/sdk/*"):
        proc = subprocess.run(
            ["git", "for-each-ref", "--format=%(refname)", pattern],
            cwd=common.ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
        for line in proc.stdout.splitlines():
            ref = line.strip()
            if not ref or ref.endswith("/HEAD"):
                continue
            branch = ref.split("refs/heads/")[-1].split("refs/remotes/origin/")[-1]
            refs.setdefault(branch, ref)

    # Nothing in main's ci.yml can gate a pull request whose base is an sdk branch:
    # GitHub resolves the workflow from the base branch, so a trigger, a path filter
    # or a job here for those branches is decoration that never runs -- and worse,
    # reads as coverage to whoever glances at the workflow list.
    checker.refute_grep(
        CI, r"branches:[^\n]*sdk/", "main's ci.yml triggers on no sdk branch (it could not run there)"
    )
    checker.refute_grep(CI, r"^[ \t]*sdk-ci:", "main's ci.yml holds no placeholder SDK job")
    checker.refute_grep(CI, r"^[ \t]+sdk:", "main's path-filter reports no sdk paths (main has no sdk/ dir)")

    packages = pointers = 0
    for branch, ref in sorted(refs.items()):
        pipeline = _git_show(ref, ".github/workflows/ci.yml")
        release = _git_show(ref, ".github/workflows/release.yml")
        if _git_show(ref, ".gitlab-ci.yml") is not None:
            checker.fail(f"{branch} still has .gitlab-ci.yml")
        if _git_show(ref, "scripts/sdk_release.py") is None:
            # A README-only pointer branch (go, php, swift): the package root lives
            # in the published repo, so a workflow here would gate nothing but itself.
            pointers += 1
            if pipeline is not None or release is not None:
                checker.fail(f"{branch} is a pointer branch and must carry no workflow")
            continue
        packages += 1
        # This file, not main's, is the only gate a pull request into this branch
        # can get -- and it only fires if it names the branch it lives on.
        if pipeline is None:
            checker.fail(f"{branch} has no ci.yml: its pull requests would merge unchecked")
        else:
            if "pull_request:" not in pipeline:
                checker.fail(f"{branch} ci.yml must trigger on pull_request")
            if f"branches: [{branch}]" not in pipeline:
                checker.fail(
                    f"{branch} ci.yml must name its own branch in branches: "
                    "(GitHub reads a pull request's workflows from its base branch)"
                )
            if "commitlint" not in pipeline:
                checker.fail(f"{branch} ci.yml must lint the pull request title")
            if "issue-link" in pipeline:
                checker.fail(f"{branch} ci.yml must not carry the main-only issue gate")
        if release is not None:
            if "types: [closed]" not in release:
                checker.fail(f"{branch} release.yml must trigger on pull_request closed")
            if "merged == true" not in release:
                checker.fail(f"{branch} release.yml must require a merged pull request")
            if re.search(r"^[ \t]*push:", release, re.MULTILINE):
                checker.fail(f"{branch} release.yml must not trigger on push")
            if "gitlab" in release.lower():
                checker.fail(f"{branch} release.yml still mentions GitLab")
    if not refs:
        checker.note("note sdk/* branch assertions skipped (no sdk/* refs in this clone)")
    else:
        checker.ok(f"checked {packages} sdk/* package branches, {pointers} pointer branches")


def check_workflows(argv: list[str]) -> int:
    common.force_lf()
    checker = Checker()
    for step in (
        _check_publish_is_manual,
        _check_workflow_expressions,
        _check_build_info_stamp,
        _check_no_emulation,
        _check_matrix_and_wiring,
        _check_asset_name_contract,
        _check_layout_invariants,
        _check_cgo_include_roots,
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
