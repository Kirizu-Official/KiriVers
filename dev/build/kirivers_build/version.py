"""Next server version derivation (D8/D9) — the release pipeline's decision core.

`next-version --format env` is `eval`-ed by release.yml and re-read by notes.py, so
stdout is a byte contract (C3): exactly four `KIRIVERS_*` lines, no quoting, no extra
bytes. Nothing else reaches stdout here; the only diagnostic this command can print is
the broken-git-range failure (D4), on stderr.
"""

from __future__ import annotations

import os
import re
import subprocess
from pathlib import Path

from . import common

# Only an annotated `vMAJOR.MINOR.PATCH` ancestor is a release base: prerelease
# (`v1.2.3-rc1`), build-metadata and two-component (`v1.2`) tags are skipped.
SEMVER_TAG = re.compile(r"^v([0-9]+)\.([0-9]+)\.([0-9]+)$")
# `type!:` / `type(scope)!:` and a `BREAKING CHANGE:` footer line are the only
# breaking markers that count; the footer has to start a body line.
BREAKING_SUBJECT = re.compile(r"^[A-Za-z0-9]+(\([^)]+\))?!:")
BREAKING_BODY = re.compile(r"^BREAKING[ -]CHANGE:", re.M)
# The type is the leading alphanumeric run of the subject, lowercased: `feat!:` yields
# `feat`, `chore(ci):` yields `chore`, a subject starting with punctuation yields "".
LEADING_ALNUM = re.compile(r"^[A-Za-z0-9]*")

MERGE_SUBJECTS = ("Merge pull request", "Merge branch", "Merge remote-tracking branch")
RANK_PATCH = 1
RANK_FEATURE = 2
RANK_BREAKING = 3
PATCH_TYPES = ("fix", "perf", "revert")

# 0x1F separates fields and 0x1E separates records; neither can appear in a commit
# message, so one `git log` pass carries hash, subject and body instead of three git
# processes per commit.
UNIT = "\x1f"
RECORD = "\x1e"
RANK_FIELDS = 3
RANK_FORMAT = RECORD + "%H" + UNIT + "%s" + UNIT + "%b"


def _git(root: Path | None, *args: str, capture_stderr: bool = False) -> subprocess.CompletedProcess:
    """Run git in `root`: stdout captured, stderr captured only when asked (keyword-only).

    Child noise must never reach our stdout (C12); `capture_stderr` exists where the
    shell original redirected git's stderr — either to quote it in a message (D4) or
    to keep a probe quiet (the tag list).
    """
    try:
        return subprocess.run(
            ["git", *args],
            cwd=str(root) if root else None,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE if capture_stderr else None,
            text=True,
            encoding="utf-8",
            errors="replace",
            check=False,
        )
    except OSError as exc:  # git missing from PATH
        return subprocess.CompletedProcess(["git", *args], 127, "", str(exc))


def _commit_rank(subject: str, body: str) -> int:
    """Rank one commit: 3 breaking, 2 feature, 1 patchable, 0 releases nothing."""
    if subject.startswith(MERGE_SUBJECTS):
        return 0
    if BREAKING_SUBJECT.match(subject) or BREAKING_BODY.search(body):
        return RANK_BREAKING
    commit_type = LEADING_ALNUM.match(subject).group(0).lower()
    if commit_type == "feat":
        return RANK_FEATURE
    if commit_type in PATCH_TYPES:
        return RANK_PATCH
    return 0


def _max_rank(root: Path, rev_range: str) -> tuple[int, str]:
    """Highest rank in `rev_range`; `(-1, git's stderr)` when git cannot read it."""
    proc = _git(
        root, "log", "--no-merges", f"--format={RANK_FORMAT}", rev_range, capture_stderr=True
    )
    if proc.returncode != 0:
        return -1, (proc.stderr or "").strip()
    max_rank = 0
    for record in proc.stdout.split(RECORD)[1:]:
        fields = record.split(UNIT)
        if len(fields) < RANK_FIELDS:
            continue
        # An empty `%b` still leaves git's newline behind, unlike the shell's `$(...)`.
        max_rank = max(max_rank, _commit_rank(fields[1], fields[2].rstrip("\n")))
    return max_rank, ""


def _previous_tag(root: Path) -> tuple[str, tuple[int, int, int]] | None:
    """Highest annotated semver tag that is an ancestor of HEAD."""
    # 'v*' stays git's own pattern: with no shell in the way it reaches `git tag`
    # unexpanded, exactly as the quoted argument did in the shell original.
    listed = _git(root, "tag", "--list", "v*", "--merged", "HEAD", capture_stderr=True)
    if listed.returncode != 0:
        # The original swallowed this (`|| true`) and fell through to 0.1.0.
        return None
    best: tuple[tuple[int, int, int], str] | None = None
    for name in listed.stdout.split():
        match = SEMVER_TAG.match(name)
        if match is None:
            continue
        kind = _git(root, "for-each-ref", "--format=%(objecttype)", f"refs/tags/{name}")
        if kind.stdout.strip() != "tag":  # lightweight tags are not release bases
            continue
        triple = tuple(int(part) for part in match.groups())
        # Highest numeric triple wins, independent of taggerdate or commit order.
        if best is None or triple > best[0]:
            best = (triple, name)
    if best is None:
        return None
    return best[1], best[0]


def _emit(fmt: str, release: str, version: str, prev: str) -> None:
    """C3: env mode always prints these four lines; plain mode prints nothing on `no`."""
    if fmt == "env":
        print(f"KIRIVERS_RELEASE={release}")
        print(f"KIRIVERS_VERSION={version}")
        print(f"KIRIVERS_TAG=v{version}")
        print(f"KIRIVERS_PREV={prev}")
    elif release == "yes":
        print(version)


def next_version(argv: list[str]) -> int:
    # Deliberately lossy, exactly like the shell original: `--format` counts only as
    # argv[0], and any unrecognised value (`--format=json`, `--help` as a value, extra
    # positionals) silently means plain mode with exit 0.
    fmt = "plain"
    if argv[:1] == ["--format"]:
        fmt = argv[1] if len(argv) > 1 else "plain"

    # The original did `cd $(git rev-parse --show-toplevel)`: usable from any
    # subdirectory of a repo, and outside one it failed with git's own message on
    # stderr plus a non-zero exit (the defect D7 shape) -- stderr therefore stays
    # inherited here and the exit code is git's.
    probe = _git(None, "rev-parse", "--show-toplevel")
    if probe.returncode != 0:
        return probe.returncode or common.FAIL_EXIT
    root = Path(os.path.normpath(probe.stdout.strip()))

    previous = _previous_tag(root)
    if previous is None:
        # Case A: no usable base, so the first release is 0.1.0 even for docs-only.
        _emit(fmt, "yes", "0.1.0", "")
        return 0
    prev_tag, (major, minor, patch) = previous

    rev_range = f"{prev_tag}..HEAD"
    rank, git_error = _max_rank(root, rev_range)
    if rank < 0:
        # D4: a bad or shallow range used to be reported as "nothing releasable" (0).
        common.log(f"next-version: git range {rev_range} failed: {git_error}")
        return common.FAIL_EXIT
    if rank == 0:
        _emit(fmt, "no", f"{major}.{minor}.{patch}", prev_tag)
        return 0
    if rank == RANK_BREAKING:
        major, minor, patch = major + 1, 0, 0
    elif rank == RANK_FEATURE:
        minor, patch = minor + 1, 0
    else:
        patch += 1
    _emit(fmt, "yes", f"{major}.{minor}.{patch}", prev_tag)
    return 0
