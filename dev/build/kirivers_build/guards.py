"""Pull-request guards: Conventional Commits, the linked-issue gate, the verdict.

Contract ids C5/C6/C7/C11 (design.md §4) name the byte promises the workflows
capture from these three subcommands:

* `issue-link` writes exactly one of `ok` | `fail` | `skip` | `error` to stdout,
* `guard` writes exactly one of `skip` | `pass` | `comment` | `close`,
* the comment file handed to `gh` starts with the hidden marker as its first
  bytes, because the workflow finds the existing comment with `startswith`.

Everything else — reasons, remediation, diagnostics — goes to stderr. The
`error` token is the D5 addition: it separates "we know nothing is linked" from
"our own lookup did not complete", so an API blip can never close a contributor's
pull request.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import traceback
from pathlib import Path

from . import common

# ------------------------------------------------------------------- commitlint

# Pinned so every job resolves the same CLI and ruleset (as the shell did); the
# Conventional Commits ruleset stays @commitlint/config-conventional's job — a
# Python re-implementation would drift (research §1.5). One version constant,
# because the tool directory name is derived from it.
COMMITLINT_VERSION = "19.8.1"
COMMITLINT_PACKAGES = (
    f"@commitlint/cli@{COMMITLINT_VERSION}",
    f"@commitlint/config-conventional@{COMMITLINT_VERSION}",
)

# Name of the config this CLI writes next to the installed packages. It is not
# the tracked commitlint.config.cjs; _commitlint_config_text explains why the
# loader has to be handed a file that sits inside the tool directory.
COMMITLINT_GENERATED_CONFIG = "kirivers.commitlint.config.cjs"

COMMITLINT_USAGE = """usage: kirivers.py commitlint
       kirivers.py commitlint --self-check
Env: COMMITLINT_TITLE  pull request title, linted over stdin when set
     COMMITLINT_FROM   git range start (exclusive, commitlint --from)
     COMMITLINT_TO     git range end (inclusive)
Push events report before/after, where before is all zeros on a new branch and
unreachable after a force push; both degrade to the tip commit only.
Exit: 0 verdict-pass, 1 verdict-fail, 2 the tool itself could not run. A 2 is
never a verdict, so pr-guard must not police the pull request over it.
Commitlint is installed on demand into <os-cache>/kirivers/commitlint-<version>,
where <os-cache> is %LOCALAPPDATA% on Windows, ~/Library/Caches on macOS, and
$XDG_CACHE_HOME or ~/.cache elsewhere. The directory name carries the version, so
a pin bump can never load the previous version's tree."""

# A `before` of 40 zeros is how GitHub says "the branch did not exist yet".
_ZERO_BEFORE = "0" * 40


def _tool(name: str) -> str | None:
    """Absolute path of a child executable, or None.

    Windows CreateProcess only appends `.exe`, so `.cmd` shims such as `npm`
    have to be resolved through PATH by us instead.
    """
    return shutil.which(name)


def _spawn(
    command: list[str], stdin_bytes: bytes | None = None, *, quiet: bool = False
) -> int:
    """Run a child, diverting its stdout into our stderr (C12).

    commitlint's human report must never land on our stdout, and stdin is given
    as bytes so a non-ASCII commit subject survives every host locale. A child we
    cannot even start reports 127, the shell's "command not found", so the caller
    reads it as a tooling fault instead of a lint verdict (D5). `quiet` throws the
    child's output away, which is what the health probe wants: its stdout is the
    whole resolved config, and 150 lines of it per run is noise, not a report.
    """
    try:
        proc = subprocess.run(
            command,
            cwd=str(common.ROOT),
            input=stdin_bytes,
            stdout=sys.stderr if not quiet else subprocess.DEVNULL,
            check=False,
        )
    except OSError as error:
        common.log(f"commitlint: cannot run {command[0]}: {error}")
        return 127
    return proc.returncode


def _git(args: list[str]) -> bytes:
    git = _tool("git") or "git"
    try:
        proc = subprocess.run(
            [git] + args, cwd=str(common.ROOT), capture_output=True, check=False
        )
    except OSError:
        return b""
    return proc.stdout if proc.returncode == 0 else b""


def _git_verify(revision: str) -> bool:
    """`git rev-parse --verify -q <revision>` as a boolean."""
    git = _tool("git") or "git"
    try:
        proc = subprocess.run(
            [git, "rev-parse", "--verify", "-q", revision],
            cwd=str(common.ROOT),
            capture_output=True,
            check=False,
        )
    except OSError:
        return False
    return proc.returncode == 0


def _cache_root() -> Path:
    """The OS cache directory for maintainer-side tool downloads.

    Deliberately outside the repository: this worktree is shared, `go build` and
    `git status` have no business seeing a node_modules tree, and a warm cache
    makes the second and later runs in a day cost one probe instead of an install.
    """
    if os.name == "nt":
        local = os.environ.get("LOCALAPPDATA")
        return Path(local) if local else Path.home() / "AppData" / "Local"
    if sys.platform == "darwin":
        return Path.home() / "Library" / "Caches"
    xdg = os.environ.get("XDG_CACHE_HOME")
    return Path(xdg) if xdg else Path.home() / ".cache"


def _commitlint_dir() -> Path:
    """Version-keyed home for the pinned commitlint tree (see COMMITLINT_USAGE)."""
    return _cache_root() / "kirivers" / f"commitlint-{COMMITLINT_VERSION}"


def _commitlint_paths(directory: Path) -> tuple[Path, Path]:
    """The two files one commitlint run needs inside an installed tree."""
    # npm's .bin shim is a `.cmd` on Windows, and an absolute path to it is what
    # CreateProcess will actually run (see _tool).
    shim = "commitlint.cmd" if os.name == "nt" else "commitlint"
    return (
        directory / "node_modules" / ".bin" / shim,
        directory / COMMITLINT_GENERATED_CONFIG,
    )


def _commitlint_config_text() -> str:
    """The generated config: a delegation, never a restatement of any rule.

    @commitlint/load resolves a bare `extends` name from the directory of the file
    it was handed (--config wins over discovery), falling back only to the global
    npm prefix. That is why `npx --package` could never work here: it installs to
    ~/.npm/_npx/<hash>/node_modules while the tracked config sits at the repo root,
    which has no node_modules at all, so a clean runner died on MODULE_NOT_FOUND and
    exit 1 read as "your title is wrong". Requiring the tracked file from inside the
    tool directory fixes the resolution base and keeps commitlint.config.cjs the only
    place the ruleset is written down.
    """
    tracked = json.dumps(str(common.ROOT / "commitlint.config.cjs"))
    return f"module.exports = require({tracked});\n"


def _write_commitlint_config(directory: Path) -> bool:
    """Refresh the generated config, but only when it changed (or is missing).

    Reports an unwritable target as False instead of raising: `cli.main()` has no
    exception handler, so an escaping OSError would exit 1 -- the very code
    pr-guard reads as "your title is wrong" and closes a conforming pull request
    over. The caller turns a False into the 2 it is contractually supposed to
    return (R4/AC5).
    """
    config = directory / COMMITLINT_GENERATED_CONFIG
    text = _commitlint_config_text()
    try:
        if config.is_file() and config.read_text(encoding="utf-8") == text:
            return True
        common.write_lf(config, text)
    except OSError as error:
        common.log(f"commitlint: cannot write {config}: {error}")
        return False
    return True


def _probe_commitlint(directory: Path) -> int:
    """0 when the pinned commitlint loads its ruleset; anything else is a fault.

    `--print-config` runs the whole config load, including the `extends` lookup that
    used to fail, and never judges a commit. That separation is the point: commitlint
    rethrows any load error and Node exits 1, which is exactly what a real rule
    violation exits with, so without this probe a broken install would be reported as
    a bad pull request title (and closed by pr-guard).
    """
    binary, config = _commitlint_paths(directory)
    if not binary.is_file() or not config.is_file():
        return 127
    return _spawn([str(binary), "--config", str(config), "--print-config"], b"", quiet=True)


def _install_commitlint(directory: Path) -> int:
    """One pinned tree into `directory`, with no package.json or lockfile left behind."""
    npm = _tool("npm")
    if npm is None:
        common.log("npm is required for commitlint")
        return 127
    return _spawn(
        [
            npm,
            "install",
            "--prefix",
            str(directory),
            "--no-save",
            "--no-package-lock",
            "--no-audit",
            "--no-fund",
            "--loglevel",
            "error",
            *COMMITLINT_PACKAGES,
        ]
    )


def _ensure_commitlint() -> Path | None:
    """Directory holding a working commitlint, or None (the caller's tooling fault).

    The install goes into a sibling temp directory and is published by rename, so a
    concurrent run -- a second commitlint invocation in one CI job, or a parallel
    session on this worktree sharing the cache -- can never observe a half-installed
    tree: it either sees nothing or a complete one. Ours is probed before it is
    published, so a broken tree is discarded rather than cached.
    """
    target = _commitlint_dir()
    if not (common.ROOT / "commitlint.config.cjs").is_file():
        # The generated config requires this file, so a probe failure would be
        # reported as an install problem and cost a needless reinstall. One line
        # here beats a Node MODULE_NOT_FOUND stack trace in the job log.
        common.log("commitlint: commitlint.config.cjs is missing; the ruleset has no source")
        return None
    try:
        target.mkdir(parents=True, exist_ok=True)
    except OSError as error:
        common.log(f"commitlint: cannot create {target}: {error}")
        return None
    # Written before the probe: the probe loads this file, and rewriting it keeps a
    # cache that outlived a moved checkout honest instead of mysteriously broken.
    if not _write_commitlint_config(target):
        return None
    if _probe_commitlint(target) == 0:
        return target

    staging = target.parent / f"{target.name}.tmp-{os.getpid()}"
    shutil.rmtree(staging, ignore_errors=True)
    try:
        if _install_commitlint(staging):
            common.log("commitlint: npm install failed")
            shutil.rmtree(staging, ignore_errors=True)
            return None
        if not _write_commitlint_config(staging):
            shutil.rmtree(staging, ignore_errors=True)
            return None
        if _probe_commitlint(staging):
            common.log("commitlint: the freshly installed tree does not load its ruleset")
            shutil.rmtree(staging, ignore_errors=True)
            return None
        try:
            shutil.rmtree(target, ignore_errors=True)  # only ever removes an empty/probe-failed dir
            os.rename(staging, target)
        except OSError:
            # Someone else published the same pin first; their tree is complete by
            # construction, so use it and drop ours.
            shutil.rmtree(staging, ignore_errors=True)
    finally:
        shutil.rmtree(staging, ignore_errors=True)
    return target if _probe_commitlint(target) == 0 else None


def _commitlint_run(
    directory: Path, extra: list[str], stdin_bytes: bytes | None = None
) -> int:
    """One commitlint invocation against the installed tree (C12 stdout handling)."""
    binary, config = _commitlint_paths(directory)
    return _spawn([str(binary), "--config", str(config), *extra], stdin_bytes)


def _lint_tip_only(directory: Path, to: str) -> int:
    """Lint only the tip: with no parent it is a root commit, so feed its subject."""
    if _git_verify(to + "^{commit}") and _git_verify(to + "^"):
        return _commitlint_run(directory, ["--from", to + "^", "--to", to])
    # Root commit: the shell piped `git log -1 --format=%s` straight into commitlint.
    return _commitlint_run(directory, [], _git(["log", "-1", "--format=%s", to]))


def _commitlint_self_check() -> int:
    """The offline half of the commit gate: layout, delegation, fault signalling.

    Nothing here shells out, so the suite stays runnable without npm or a network --
    which is what `_check_offline_suites` in selfcheck.py requires.
    """
    failed = 0

    def expect(ok: bool, message: str) -> None:
        nonlocal failed
        if not ok:
            common.log(f"FAIL: {message}")
            failed = 1

    directory = _commitlint_dir()
    expect(
        directory.name == f"commitlint-{COMMITLINT_VERSION}",
        "the tool directory name must carry the pinned version",
    )
    expect(
        _cache_root() in directory.parents,
        "the tool directory must sit under the OS cache root",
    )
    expect(
        common.ROOT not in directory.parents,
        "the tool directory must not live in the repository",
    )

    binary, config = _commitlint_paths(directory)
    expect(
        binary.name.endswith(".cmd") == (os.name == "nt"),
        "the npm bin shim must match the platform's variant",
    )
    expect(config.name == COMMITLINT_GENERATED_CONFIG, "the generated config name is a contract")
    expect(
        all(p.endswith(f"@{COMMITLINT_VERSION}") for p in COMMITLINT_PACKAGES),
        "every pinned package must name COMMITLINT_VERSION, or the cache key lies",
    )

    text = _commitlint_config_text()
    tracked = json.dumps(str(common.ROOT / "commitlint.config.cjs"))
    expect(
        text == f"module.exports = require({tracked});\n",
        "the generated config must delegate to the tracked file and nothing else",
    )
    for forbidden in ("extends", "rules", "parserPreset", "config-conventional"):
        expect(forbidden not in text, f"the generated config must not restate rules ({forbidden})")
    expect(
        (common.ROOT / "commitlint.config.cjs").is_file(),
        "commitlint.config.cjs is the single ruleset declaration",
    )

    workspace = Path(tempfile.mkdtemp(prefix="kirivers-commitlint-selfcheck-"))
    try:
        empty = workspace / "nothing-installed"
        empty.mkdir()
        expect(
            _write_commitlint_config(empty) is True,
            "a writable tool directory must report success",
        )
        # No tree yet: the probe must report a tooling fault, not a lint verdict.
        expect(_probe_commitlint(empty) != 0, "a missing install must not probe healthy")
        expect(
            _probe_commitlint(workspace.parent / "definitely-not-here") != 0,
            "a missing directory must not probe healthy",
        )
        written = (empty / COMMITLINT_GENERATED_CONFIG).read_text(encoding="utf-8")
        expect(written == text, "the generated config must be written verbatim")
        expect("\r" not in written, "the generated config must be LF-only")

        # R4/AC5: an unwritable config is a fault the caller turns into a 2. It must
        # not escape as a traceback, because cli.main() exits 1 on an uncaught
        # exception and pr-guard reads 1 as "bad title" and closes the pull request.
        blocked = workspace / "blocked"
        blocked.mkdir()
        (blocked / COMMITLINT_GENERATED_CONFIG).mkdir()
        expect(
            _write_commitlint_config(blocked) is False,
            "an unwritable config must be reported, not raised",
        )
    finally:
        shutil.rmtree(workspace, ignore_errors=True)

    if failed:
        common.log("commitlint: self-check FAILED")
        return common.FAIL_EXIT
    common.log("commitlint: self-check ok")
    return 0


def _commitlint_verdict(title: str, frm: str, to: str) -> int:
    """Lint the title and then the range; 0 pass, 1 verdict, 2 tooling fault."""
    directory = _ensure_commitlint()
    if directory is None:
        # Same reasoning as D5 and the message pr-guard's `error` branch exists for:
        # an install we could not complete says nothing about anyone's commit message.
        common.log("commitlint: no usable installation; this is a tooling fault, not a verdict")
        return common.USAGE_EXIT

    if title:
        status = _commitlint_run(directory, [], title.encode("utf-8") + b"\n")
        if status:
            return status  # `set -e`: a bad title never reaches the range branch

    if to:
        if not frm or frm.startswith(_ZERO_BEFORE):
            common.log(f"commitlint: no usable range start, linting only {to}")
            status = _lint_tip_only(directory, to)
        elif not _git_verify(frm + "^{commit}"):
            common.log(f"commitlint: {frm} is not reachable here, linting only {to}")
            status = _lint_tip_only(directory, to)
        else:
            status = _commitlint_run(directory, ["--from", frm, "--to", to])
        if status:
            return status
    return 0


def commitlint(argv: list[str]) -> int:
    """Env-driven wrapper around the pinned commitlint CLI.

    The .sh ignored every argument, so this does too; only --help and --self-check
    are handled, to keep the three guard subcommands discoverable from one entrypoint.
    """
    if any(arg in ("-h", "--help") for arg in argv):
        common.log(COMMITLINT_USAGE)
        return 0
    if "--self-check" in argv:
        return _commitlint_self_check()
    if _tool("npm") is None:
        # D5: a missing tool is a tooling fault (2), not a lint failure (1).
        common.log("npm is required for commitlint")
        return common.USAGE_EXIT
    title = os.environ.get("COMMITLINT_TITLE", "")
    to = os.environ.get("COMMITLINT_TO", "")
    frm = os.environ.get("COMMITLINT_FROM", "")
    if not title and not to:
        # Checked before anything is installed: without a title or a range there is
        # no verdict to compute, and a 20-second npm install would only bury the
        # "set COMMITLINT_TITLE and/or COMMITLINT_TO" hint this returns for.
        common.log("set COMMITLINT_TITLE and/or COMMITLINT_TO (with optional COMMITLINT_FROM)")
        return common.USAGE_EXIT
    try:
        return _commitlint_verdict(title, frm, to)
    except Exception as error:  # noqa: BLE001 -- the exit-code contract outranks the bug
        # Every way this gate breaks on its own has to be a 2. `cli.main()` lets an
        # exception escape as exit 1, and 1 is the verdict code pr-guard polices:
        # an unwritable cache directory would have commented on and closed someone's
        # healthy pull request. The traceback still goes to stderr, so the fault
        # stays diagnosable -- it just can never be judgeable.
        common.log(f"commitlint: {type(error).__name__}: {error}")
        traceback.print_exc()
        return common.USAGE_EXIT


# -------------------------------------------------------------------- issue-link

# Verbatim KEYWORD_RE from check-issue-link.sh. Matched on BYTES, line by line,
# case-insensitively, because the shell ran `grep -Eiq` under LC_ALL=C: byte-wise,
# folding only ASCII case, and never across a newline.
KEYWORD_RE = re.compile(
    rb"(^|[ \t\n\r\f\v])(close|closes|closed|fix|fixes|fixed|resolve|resolves|resolved)"
    rb"[ \t\n\r\f\v]*:?[ \t\n\r\f\v]*#[0-9]+",
    re.IGNORECASE,
)

REMEDATION = (
    "Link an issue to this pull request:\n"
    '  - use "Development -> Link an issue" in the pull request sidebar, or\n'
    '  - write a line such as "Fixes #123" (closes / resolves work too, any casing), or\n'
    '  - if this pull request genuinely needs no issue, add the "skip-issue-check" label.'
)

ISSUE_GRAPHQL_QUERY = (
    "query($owner:String!,$name:String!,$number:Int!){"
    "repository(owner:$owner,name:$name){pullRequest(number:$number)"
    "{closingIssuesReferences{totalCount}}}}"
)
ISSUE_GRAPHQL_JQ = ".data.repository.pullRequest.closingIssuesReferences.totalCount"

ISSUE_LINK_USAGE = """usage: kirivers.py issue-link [--record <file>] [pr_number]
       kirivers.py issue-link --self-check
Env: GITHUB_REPOSITORY, KIRIVERS_ISSUE_CLOSING, KIRIVERS_ISSUE_LINK_ENFORCE
Prints one token on stdout: ok | fail | skip | error."""


def _body_references_issue(body: bytes) -> bool:
    return any(KEYWORD_RE.search(line) for line in body.split(b"\n"))


def _classify(body: bytes, closing: str, skip_label: bool, ctx: bool, lookup_failed: bool) -> str:
    """The five ordered steps of classify(), plus D5's `error`.

    D5 is applied last and only where the old code would have said `fail`: a
    verdict needs evidence, and a broken lookup is not evidence of absence.
    """
    if not ctx:
        return "skip"
    if skip_label:
        return "skip"
    if closing and closing != "0":
        return "ok"
    if _body_references_issue(body):
        return "ok"
    return "error" if lookup_failed else "fail"


def _gh_closing_count(pr: str) -> tuple[str, bool]:
    """Ask GitHub which issues this PR closes. Returns (value, tooling_failed)."""
    repo = os.environ.get("GITHUB_REPOSITORY", "")
    gh = _tool("gh")
    if gh is None:
        return "", False
    command = [
        gh,
        "api",
        "graphql",
        "-f",
        "query=" + ISSUE_GRAPHQL_QUERY,
        "-f",
        "owner=" + repo.split("/", 1)[0],
        "-f",
        "name=" + repo.rsplit("/", 1)[-1],
        "-F",
        "number=" + pr,
        "--jq",
        ISSUE_GRAPHQL_JQ,
    ]
    try:
        proc = subprocess.run(command, capture_output=True, check=False)
    except OSError as error:  # gh present on PATH but not runnable: still a fault
        common.log(f"check-issue-link: cannot run gh: {error}")
        return "", True
    value = proc.stdout.strip()
    if proc.returncode or not value.isdigit():
        # The shell swallowed this into an empty `closing`; D5 keeps it visible.
        common.log(f"check-issue-link: closing-issue lookup failed (rc={proc.returncode})")
        _log_first_line(proc.stderr, "check-issue-link: gh api graphql: ")
        return "", True
    return value.decode("ascii", "replace"), False


def _gh_fetch_body(pr: str) -> tuple[bytes, bool]:
    gh = _tool("gh")
    if gh is None:
        return b"", False
    command = [
        gh, "pr", "view", pr, "--repo", os.environ.get("GITHUB_REPOSITORY", ""),
        "--json", "body", "--jq", ".body",
    ]
    try:
        proc = subprocess.run(command, capture_output=True, check=False)
    except OSError as error:
        common.log(f"check-issue-link: cannot run gh: {error}")
        return b"", True
    if proc.returncode:
        common.log(f"check-issue-link: pull request body fetch failed (rc={proc.returncode})")
        _log_first_line(proc.stderr, "check-issue-link: gh pr view: ")
        return b"", True
    return proc.stdout + b"\n", False


def _log_first_line(payload: bytes, prefix: str) -> None:
    if payload:
        common.log(prefix + payload.split(b"\n", 1)[0].decode("utf-8", "replace"))


def _issue_self_check() -> int:
    """The shell's 10 offline fixtures, the D5 `error` rows and the D3 label row."""
    # One row per offline fixture: the shell's ten first, then the new states.
    # (expected, body, closing, skip_label, ctx, lookup_failed, label)
    fixtures = [
        ("ok", "", "1", False, True, False, "sidebar-linked issue"),
        ("ok", "Fixes #123", "0", False, True, False, "keyword + hash"),
        ("ok", "Closes: #45", "0", False, True, False, "keyword with colon"),
        ("ok", "resolves #7 in the summary", "0", False, True, False, "any casing"),
        ("ok", "first line | fixes #9 | tail", "0", False, True, False, "keyword on a joined line"),
        ("skip", "", "0", True, True, False, "skip-issue-check label"),
        ("fail", "refactors internals, no issue", "0", False, True, False, "nothing linked"),
        ("fail", "discloses: #12 and fixes nothing", "0", False, True, False, "keyword must start a word"),
        ("fail", "see #123 for context", "0", False, True, False, "bare reference is not a link"),
        ("skip", "", "0", False, False, False, "push event without a pull request"),
        # D5: a tooling fault is only an `error` when nothing else proves the link.
        ("error", "refactors internals, no issue", "", False, True, True, "lookup failed -> error"),
        ("error", "refactors internals, no issue", "0", False, True, True, "zero count from a broken call"),
        ("ok", "Fixes #123", "", False, True, True, "keyword rescues a failed lookup"),
        ("skip", "", "0", True, True, True, "label exempts a failed lookup"),
        ("skip", "", "0", False, False, True, "no context outranks a failed lookup"),
    ]
    failed = 0
    for expected, body, closing, skip, ctx, broken, label in fixtures:
        got = _classify(body.encode("utf-8"), closing, skip, ctx, broken)
        if got != expected:
            common.log(f"FAIL: {label} expected '{expected}', got '{got}'")
            failed = 1
    if failed:
        common.log("check-issue-link: self-check FAILED")
        return common.FAIL_EXIT
    common.log("check-issue-link: self-check ok")
    return 0


def issue_link(argv: list[str]) -> int:
    record = ""
    pr = ""
    check = False
    index = 0
    while index < len(argv):
        arg = argv[index]
        index += 1
        if arg == "--record":
            # A trailing --record consumes nothing and means "no record", as before.
            record = argv[index] if index < len(argv) else ""
            if record:
                index += 1
        elif arg == "--self-check":
            check = True
        elif arg in ("-h", "--help"):
            common.log(ISSUE_LINK_USAGE)
            return 0
        elif arg.startswith("-"):
            common.log(f"check-issue-link: unknown option {arg}")
            return common.USAGE_EXIT
        else:
            pr = arg

    if check:
        return _issue_self_check()

    body = b""
    skip_label = False
    record_path = Path(record) if record else None
    if record_path is not None:
        if not record_path.is_file():
            common.log(f"check-issue-link: --record file not found: {record}")
            return common.USAGE_EXIT
        body = (common.record_field("body", record_path) + "\n").encode("utf-8")
        # D3: one trimming matcher shared with `guard`.
        skip_label = common.has_label(
            common.record_field("labels", record_path), "skip-issue-check"
        )
        if not pr:
            pr = common.record_field("number", record_path)

    # A direct push to main has no pull request to gate (D15); the sdk/* base
    # exemption stays in the workflow `if:` on purpose (research §2.5).
    ctx = bool(record) or bool(pr)
    closing = os.environ.get("KIRIVERS_ISSUE_CLOSING", "")
    if record and not closing:
        closing = common.record_field("closing", record_path)
    enforce = os.environ.get("KIRIVERS_ISSUE_LINK_ENFORCE", "1") != "0"

    lookup_failed = False
    repository = os.environ.get("GITHUB_REPOSITORY", "")
    if not closing and ctx and not skip_label and _tool("gh") and repository:
        closing, broken = _gh_closing_count(pr)
        lookup_failed = lookup_failed or broken
    if ctx and record_path is None and pr and _tool("gh") and repository:
        body, broken = _gh_fetch_body(pr)
        lookup_failed = lookup_failed or broken

    token = _classify(body, closing, skip_label, ctx, lookup_failed and enforce)
    sys.stdout.write(token + "\n")

    if token == "ok":
        if closing and closing != "0":
            common.log(f"check-issue-link: {closing} linked closing issue(s) -> ok")
        else:
            common.log("check-issue-link: body references an issue -> ok")
        return 0
    if token == "skip":
        if not ctx:
            common.log("check-issue-link: no pull request context (push event) -> skip")
        else:
            common.log("check-issue-link: skip-issue-check label present -> skip")
        return 0
    if token == "error":
        common.log(
            "check-issue-link: linked-issue lookup did not complete -> error (not a verdict)"
        )
        return common.USAGE_EXIT
    # The ENFORCE=0 bypass keeps printing `fail` and exiting 0 (warning only).
    if not enforce:
        common.log("check-issue-link: no issue linked (enforcement disabled) -> warning only")
        return 0
    common.log(f"check-issue-link: no issue linked to PR {pr or 'this pull request'}")
    common.log(REMEDATION)
    return common.FAIL_EXIT


# -------------------------------------------------------------------------- guard

# C7: the workflow locates its own comment with startswith() on these bytes.
MARKER = "<!-- pr-guard -->"
# Advisory display string (commitlint's own type-enum is the real gate); copied
# verbatim, including the order, which differs from config-conventional's.
TYPES = "feat | fix | perf | revert | docs | refactor | test | style | chore | ci | build"
FOOTER = (
    "_这条留言由 `.github/workflows/pr-guard.yml` 自动维护（PR #{number}）。"
    "/ Maintained automatically by the pull request guard._"
)
ZH_TITLE_ITEM = (
    "{n}. **标题格式**：当前标题 `{title}` 不符合 Conventional Commits。"
    "改成 `<类型>(<可选 scope>): <一句话说明>`，例如 `fix(client): 修正空响应`；"
    "破坏性变更写成 `类型!:` 或在正文加 `BREAKING CHANGE:`。"
    "类型只能是 {types}，scope 用小写。"
)
ZH_ISSUE_ITEM = (
    "{n}. **绑定 Issue**：还没有关联任何 Issue。请先开一个 Issue 描述要解决的问题，"
    "然后在本 PR 描述里写 `Fixes #<编号>`（`Closes` / `Resolves` 同样有效），"
    "或用右侧栏 Development → Link an issue 关联。"
    "确实不需要 Issue 时，给本 PR 加 `skip-issue-check` 标签。"
)
EN_TITLE_ITEM = (
    "{n}. **Title format**: `{title}` is not Conventional Commits. "
    "Use `<type>(<optional scope>): <summary>`, e.g. `fix(client): handle empty response`. "
    "Mark breaking changes with `type!:` or a `BREAKING CHANGE:` footer. "
    "Allowed types: {types}. Keep the scope lowercase."
)
EN_ISSUE_ITEM = (
    "{n}. **Linked issue**: nothing is linked yet. Open an issue describing the problem, "
    "then write `Fixes #<number>` (Closes / Resolves also work) in this pull request body, "
    "or use Development -> Link an issue. If an issue really is not needed, "
    "add the `skip-issue-check` label."
)
ZH_CLOSE_TAIL = (
    "处理完上面两项后，点 **Reopen pull request** 重新打开即可，"
    "守卫会自动复检并把这条留言更新为通过提示；不必重开一个新的 PR。"
)
ZH_COMMENT_TAIL = "请补齐上面各项，维护者合并前会再检查一次。"
EN_CLOSE_TAIL = (
    "Once both are fixed, click **Reopen pull request** — the guard re-checks it and turns "
    "this comment into a pass notice; you do not need to open a new pull request."
)
EN_COMMENT_TAIL = "Please address the items above; a maintainer will re-check before merging."

GUARD_USAGE = """usage: kirivers.py guard --record <file> [--title-ok 1|0|error]
                                  [--issue-ok 1|0|error] [--out <file>]
       kirivers.py guard --self-check
Record keys (one `key=value` per line): number, draft, title, body, author,
typename, association, labels, closing.
Prints one decision token on stdout: skip | pass | comment | close."""


def _pass_body(record: Path) -> str:
    return (
        MARKER + "\n"
        "✅ 检查已通过 / Checks passed\n\n"
        "标题符合 Conventional Commits，Issue 也已关联。\n"
        "The title follows Conventional Commits and an issue is linked.\n\n"
        + FOOTER.format(number=common.record_field("number", record)) + "\n"
    )


def _comment_body(record: Path, title_ok: bool, issue_ok: bool, will_close: bool) -> str:
    title = common.record_field("title", record)
    parts = [MARKER + "\n"]
    if will_close:
        parts.append(
            "### ⛔ 未通过自动检查，已自动关闭 / Auto-closed: this pull request did not "
            "pass the automated checks\n\n"
        )
    else:
        parts.append(
            "### ⚠️ 未通过自动检查 / This pull request did not pass the automated checks\n\n"
        )
    parts.append("| 检查 / Check | 结果 / Result |\n| --- | --- |\n")
    parts.append(
        "| PR 标题符合 Conventional Commits / title format | %s |\n" % ("✅" if title_ok else "❌")
    )
    # Quirk kept knowingly (research §3.3): this row shows the raw issue check, not
    # the label-waived verdict, so a waived issue can still read ❌ here.
    parts.append("| 已绑定 Issue / linked issue | %s |\n" % ("✅" if issue_ok else "❌"))
    parts.append("\n**中文**\n\n")
    step = 0
    if not title_ok:
        step += 1
        parts.append(ZH_TITLE_ITEM.format(n=step, title=title, types=TYPES) + "\n")
    if not issue_ok:
        step += 1
        parts.append(ZH_ISSUE_ITEM.format(n=step) + "\n")
    parts.append("\n" + (ZH_CLOSE_TAIL if will_close else ZH_COMMENT_TAIL) + "\n\n")
    # The counter restarts per language block, exactly as the shell's two n=0 did.
    parts.append("**English**\n\n")
    step = 0
    if not title_ok:
        step += 1
        parts.append(EN_TITLE_ITEM.format(n=step, title=title, types=TYPES) + "\n")
    if not issue_ok:
        step += 1
        parts.append(EN_ISSUE_ITEM.format(n=step) + "\n")
    parts.append("\n" + (EN_CLOSE_TAIL if will_close else EN_COMMENT_TAIL) + "\n\n")
    parts.append(FOOTER.format(number=common.record_field("number", record)) + "\n")
    return "".join(parts)


def _decide(record: Path, title_ok: str, issue_ok: str, out: Path) -> str:
    """Return the decision token and write the body `out` unless skipping."""
    # D5: an axis we could not measure is not a failure, so it outranks the whole
    # table and deliberately leaves --out untouched.
    if "error" in (title_ok, issue_ok):
        common.log(
            f"pr-guard: check results incomplete (title-ok={title_ok}, issue-ok={issue_ok});"
            " skipping without a verdict"
        )
        return "skip"

    draft = common.record_field("draft", record)
    typename = common.record_field("typename", record)
    author = common.record_field("author", record)
    association = common.record_field("association", record)
    labels = common.record_field("labels", record)

    # Exemptions (D16): drafts are work in progress, bots must not be policed.
    if draft in ("true", "True"):
        return "skip"
    if typename == "Bot":
        return "skip"
    if author.endswith("[bot]"):
        return "skip"

    # Anything that is not "1" reads as "that check did not pass" (fail-closed).
    title_passed = title_ok == "1"
    issue_passed = issue_ok == "1"
    linked = issue_passed or common.has_label(labels, "skip-issue-check")
    if title_passed and linked:
        common.write_lf(out, _pass_body(record))
        return "pass"
    if association in ("OWNER", "MEMBER", "COLLABORATOR"):
        common.write_lf(out, _comment_body(record, title_passed, issue_passed, False))
        return "comment"
    common.write_lf(out, _comment_body(record, title_passed, issue_passed, True))
    return "close"


def _fixture_record(
    directory: Path, draft: str, author: str, typename: str, association: str, labels: str
) -> Path:
    """A production-shaped record, same keys and order as pr-guard.yml writes."""
    path = directory / "rec"
    common.write_lf(
        path,
        "number=42\n"
        f"draft={draft}\n"
        f"author={author}\n"
        f"typename={typename}\n"
        f"association={association}\n"
        f"labels={labels}\n"
        "title=feat: something\n"
        "body=nothing linked here\n",
    )
    return path


def _guard_self_check() -> int:
    failed = 0
    workspace = Path(tempfile.mkdtemp(prefix="kirivers-guard-selfcheck-"))
    try:
        out = workspace / "out"

        def expect(expected, draft, author, typename, association, labels, tval, ival):
            nonlocal failed
            got = _decide(
                _fixture_record(workspace, draft, author, typename, association, labels),
                tval,
                ival,
                out,
            )
            if got != expected:
                common.log(
                    f"FAIL: expected '{expected}', got '{got}' (draft={draft} author={author}"
                    f" typename={typename} assoc={association} labels={labels}"
                    f" title_ok={tval} issue_ok={ival})"
                )
                failed = 1

        # Draft and bot authors are exempt before anything else.
        expect("skip", "true", "alice", "User", "NONE", "", "0", "0")
        # The shell's fixture spells this one typename=OWNER, association empty.
        expect("skip", "true", "alice", "OWNER", "", "", "0", "0")
        expect("skip", "false", "renovate", "Bot", "NONE", "", "0", "0")
        expect("skip", "false", "github-actions[bot]", "User", "NONE", "", "1", "1")
        # Passing needs both checks (or the exemption label).
        expect("pass", "false", "alice", "User", "OWNER", "", "1", "1")
        expect("pass", "false", "alice", "User", "CONTRIBUTOR", "skip-issue-check", "1", "0")
        expect("pass", "false", "alice", "User", "FIRST_TIMER", "", "1", "1")
        # D3: the padded label now counts on both sides, so this passes too.
        expect("pass", "false", "alice", "User", "CONTRIBUTOR", "area-docs, skip-issue-check", "1", "0")
        # Maintainer-side authors get the comment but keep the PR open.
        expect("comment", "false", "alice", "User", "OWNER", "", "0", "1")
        expect("comment", "false", "alice", "User", "MEMBER", "", "1", "0")
        expect("comment", "false", "alice", "User", "COLLABORATOR", "", "0", "0")
        # Everyone else (NONE / FIRST_TIMER / CONTRIBUTOR / MANNEQUIN) is closed.
        expect("close", "false", "carol", "User", "NONE", "", "0", "1")
        expect("close", "false", "carol", "User", "FIRST_TIMER", "", "0", "0")
        expect("close", "false", "carol", "User", "CONTRIBUTOR", "", "1", "0")
        expect("close", "false", "mangod", "User", "", "", "0", "0")
        # D5: an unavailable axis skips and writes nothing, even for a closable PR.
        for tval, ival in (("error", "1"), ("1", "error"), ("error", "error")):
            out.unlink(missing_ok=True)
            expect("skip", "false", "carol", "User", "NONE", "", tval, ival)
            if out.exists():
                common.log(f"FAIL: error axis ({tval},{ival}) still wrote the --out file")
                failed = 1

        record = _fixture_record(workspace, "false", "carol", "User", "NONE", "area-docs")
        _decide(record, "0", "0", out)
        text = out.read_text(encoding="utf-8")
        for needle in (
            MARKER,
            "Reopen pull request",
            "skip-issue-check",
            "Conventional Commits",
            "绑定 Issue",
            "Linked issue",
            "Fixes #",
        ):
            if needle not in text:
                common.log(f"FAIL: close comment body is missing '{needle}'")
                failed = 1
        if text.startswith("\ufeff") or not text.startswith(MARKER):
            common.log("FAIL: comment body must start with the marker, byte 0 (C7)")
            failed = 1
        if "\r" in text:
            common.log("FAIL: comment body must be LF-only")
            failed = 1
        if "1. **Title format**" not in text and "2. **Title format**" not in text:
            common.log("FAIL: both items must be numbered when both checks fail")
            failed = 1
        # Only the issue check failing -> a single numbered item per language.
        _decide(record, "1", "0", out)
        text = out.read_text(encoding="utf-8")
        if "2. **Linked issue**" in text:
            common.log("FAIL: item numbering must restart per language")
            failed = 1
        if "1. **Linked issue**" not in text:
            common.log("FAIL: missing the linked-issue item")
            failed = 1
        # A comment-only decision must not read like a closure.
        _decide(_fixture_record(workspace, "false", "alice", "User", "MEMBER", ""), "0", "1", out)
        if "Auto-closed" in out.read_text(encoding="utf-8"):
            common.log("FAIL: maintainer comment must not say auto-closed")
            failed = 1
        # Pass keeps the marker so the workflow can edit the old comment.
        _decide(_fixture_record(workspace, "false", "alice", "User", "OWNER", ""), "1", "1", out)
        if not out.read_text(encoding="utf-8").startswith(MARKER):
            common.log("FAIL: pass body must keep the hidden marker")
            failed = 1
    finally:
        shutil.rmtree(workspace, ignore_errors=True)

    if failed:
        common.log("pr-guard: self-check FAILED")
        return common.FAIL_EXIT
    common.log("pr-guard: self-check ok")
    return 0


def pr_guard(argv: list[str]) -> int:
    record = ""
    title_ok = "1"
    issue_ok = "1"
    out = ""
    check = False
    index = 0
    while index < len(argv):
        arg = argv[index]
        index += 1
        # A bare --title-ok/--issue-ok/--out at end-of-args keeps its default,
        # which is what ${1:-1} / ${1:-} did in the shell.
        if arg == "--record":
            record = argv[index] if index < len(argv) else ""
            index += 1
        elif arg == "--title-ok":
            title_ok = argv[index] if index < len(argv) else "1"
            index += 1
        elif arg == "--issue-ok":
            issue_ok = argv[index] if index < len(argv) else "1"
            index += 1
        elif arg == "--out":
            out = argv[index] if index < len(argv) else ""
            index += 1
        elif arg == "--self-check":
            check = True
        elif arg in ("-h", "--help"):
            common.log(GUARD_USAGE)
            return 0
        else:
            common.log(f"pr-guard: unknown argument {arg}")
            return common.USAGE_EXIT

    if check:
        return _guard_self_check()

    record_path = Path(record) if record else None
    if record_path is None or not record_path.is_file():
        common.log("pr-guard: --record <file> is required")
        return common.USAGE_EXIT

    if out:
        out_path = Path(out)
        scratch = None
    else:
        scratch = Path(tempfile.mkdtemp(prefix="kirivers-guard-"))
        out_path = scratch / "comment.md"
    try:
        decision = _decide(record_path, title_ok, issue_ok, out_path)
    finally:
        if scratch is not None:
            shutil.rmtree(scratch, ignore_errors=True)
    sys.stdout.write(decision + "\n")
    return 0
