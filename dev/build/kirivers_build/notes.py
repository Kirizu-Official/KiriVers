"""Release notes and the CHANGELOG splice.

Sections come from the commit `type`, entries are prefixed with its `scope`,
and the emoji, label and section order come from `changelog-types.conf`. This
module only decides what is *displayed*: version rank comes from `next-version`,
which never reads the type table (AC14 asserts the two stay independent).

stdout contract (C8): `release-notes` emits only the notes body, whose first
line is `## v<ver> — <date>` with an U+2014 dash and one space each side, because
`changelog` reads the version back out of it. `changelog` and `--self-check`
emit zero bytes on stdout; every diagnostic goes to stderr (C12).

Unlike the shell original, nothing here is passed through a quoting layer that
can mangle a path, so the type table is read on Windows too (defect D7).
"""

from __future__ import annotations

import contextlib
import io
import os
import re
import shutil
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path

from . import common

# 0x1F separates the fields of one `git log` record; it cannot occur in a
# subject, so a single log pass carries short hash, full hash and subject.
UNIT = "\x1f"
EM_DASH = "\u2014"
RECORD_FIELDS = 3
BREAKING_GREP = "--grep=^BREAKING[ -]CHANGE:"

# Synthetic buckets that never collide with a config key (keys match KEY_RE).
BREAKING_BUCKET = "__breaking__"
OTHER_BUCKET = "__other__"
BREAKING_EMOJI = "\U0001F4A5"  # 💥
BREAKING_LABEL = "Breaking Changes"
OTHER_EMOJI = "\U0001F516"  # 🔖
OTHER_LABEL = "Other"

EMPTY_NOTICE = "No user-facing Conventional Commits in this range."
BASELINE_LINE = (
    "Initial release baseline: no previous annotated `v*` tag is an ancestor "
    "of {head}, so earlier commits are not replayed."
)

KEY_RE = re.compile(r"[a-z][a-z0-9-]*")
# A `#` line only hides a type when the rest looks like `type=`; any other
# comment is prose (this is why the shipped file's Chinese header is inert).
HIDDEN_RE = re.compile(r"[ \t]*[a-z][a-z0-9-]*[ \t]*=")
SEP_RE = re.compile(r"[ \t=]")
PR_TAIL_RE = re.compile(r"\(#[0-9]+\)[ \t]*$")
TYPE_RE = re.compile(r"[a-zA-Z][a-zA-Z0-9-]*")
MERGE_PREFIXES = ("Merge pull request", "Merge branch", "Merge remote-tracking branch")
HEADING_VERSION_RE = re.compile(r"## ([^ \t]*)")

DEFAULT_CONF_NAME = "dev/build/changelog-types.conf"

# Embedded fallback table; must parse identically to the shipped conf file.
DEFAULT_CONF_TEXT = """breaking=💥=Breaking Changes
feat=✨=Features
fix=🐛=Bug Fixes
perf=⚡=Performance
revert=⏪=Reverts
refactor=♻️=Refactoring
# docs=📝=Documentation
# test=✅=Tests
# style=💄=Style
# chore=🔧=Chores
# ci=👷=CI
# build=📦=Build
"""

USAGE = """usage: kirivers.py release-notes [options] [prev_tag] [head]
  --version <semver>  heading version (default: next-version result)
  --config <file>     type table (default dev/build/changelog-types.conf)
  --pr-map <file>     lines "<sha7> <pr>" instead of the GitHub API
  --date <text>       fixed heading date (used by --self-check)
  --self-check        offline fixture assertions for the notes format and AC14

The type table is display-only: version rank comes from `next-version`, which
never reads it."""

_VALUE_FLAGS = ("--version", "--config", "--pr-map", "--date")


class _Abort(Exception):
    """A failure whose exit code must reach the caller (C11): git's own code when
    a log read fails, with `message` for the self-check to report."""

    def __init__(self, code: int, message: str = "") -> None:
        super().__init__(message or f"aborted with exit code {code}")
        self.code = code
        self.message = message


class _ConfError(_Abort):
    """Malformed type table: usage exit 2, every message already on stderr."""

    def __init__(self) -> None:
        super().__init__(common.USAGE_EXIT)


class _Types:
    """Parsed type table: section order plus the emoji/label of each active key."""

    def __init__(self) -> None:
        self.order: list[str] = []
        self.emoji: dict[str, str] = {}
        self.label: dict[str, str] = {}
        self.hidden: set[str] = set()


def _trim(text: str) -> str:
    """awk's trim(): spaces, tabs and stray CRs at both ends, never a newline."""
    return text.strip(" \t\r")


def _sep_index(text: str) -> int:
    found = SEP_RE.search(text)
    return -1 if found is None else found.start()


def _git_text(repo: Path, *args: str, fatal: bool = True) -> str:
    """Run git with stdout captured; stderr stays inherited so git's own report
    is visible, and a failed read aborts the render like the shell's `set -e`."""
    try:
        proc = subprocess.run(
            ["git", *args],
            cwd=str(repo),
            stdout=subprocess.PIPE,
            stderr=None,
            text=True,
            encoding="utf-8",
            errors="replace",
            check=False,
        )
    except OSError as error:
        raise _Abort(common.FAIL_EXIT, f"cannot run git {args[0] if args else ''}: {error}") from error
    if proc.returncode != 0:
        if not fatal:
            return ""
        raise _Abort(proc.returncode or common.FAIL_EXIT)
    return proc.stdout or ""


def _parse_types(text: str) -> _Types:
    """One row per type: `type=emoji[=label]`, file order == section order.

    Every rule is the awk parser's, including the odd ones: a row without `=`
    has an empty key, and a single-token value becomes both emoji and label.
    """
    types = _Types()
    bad = False
    for line in text.split("\n"):
        line = line[:-1] if line.endswith("\r") else line
        if _trim(line) == "":
            continue
        body = line
        hidden = False
        if line.startswith("#"):
            rest = line[1:]
            if HIDDEN_RE.match(rest) is None:
                continue
            body = rest
            hidden = True
        eq = body.find("=")
        if eq < 0:
            key, val = "", _trim(body)
        else:
            key, val = _trim(body[:eq]), _trim(body[eq + 1:])
        if KEY_RE.fullmatch(key) is None:
            common.log(f"changelog-types: invalid type key in: {line}")
            bad = True
            continue
        if val == "":
            common.log(f"changelog-types: missing value for {key}")
            bad = True
            continue
        sep = _sep_index(val)
        if sep < 0:
            emoji, label = val, key
        else:
            emoji = _trim(val[:sep])
            label = _trim(val[sep + 1:]) or key
        if emoji == "" or SEP_RE.search(emoji) is not None:
            common.log(f"changelog-types: emoji must be a single token for {key}")
            bad = True
            continue
        if hidden:
            if key in types.emoji:
                common.log(f"changelog-types: {key} is both active and commented out")
                bad = True
                continue
            types.hidden.add(key)
            continue
        if key in types.emoji:
            common.log(f"changelog-types: duplicate type {key}")
            bad = True
            continue
        if key in types.hidden:
            common.log(f"changelog-types: {key} is both active and commented out")
            bad = True
            continue
        types.order.append(key)
        types.emoji[key] = emoji
        types.label[key] = label
    if bad:
        raise _ConfError()
    return types


def _types_for(conf: Path | None) -> _Types:
    """Load the shipped table, or the embedded one when the file is not there."""
    if conf is None:
        text = DEFAULT_CONF_TEXT
    elif conf.is_file():
        text = conf.read_text(encoding="utf-8", errors="replace")
    else:
        # as_posix() keeps the caller's own spelling of the path, as the shell's
        # message did; str(Path) would rewrite the separators on Windows.
        common.log(
            f"release-notes: {conf.as_posix()} missing, "
            "falling back to the embedded default table"
        )
        text = DEFAULT_CONF_TEXT
    return _parse_types(text)


def _read_pr_map(path: Path) -> dict[str, str]:
    """`<sha-abbrev> <pr>` per line, split on the first space."""
    mapping: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8", errors="replace").split("\n"):
        line = _trim(line)
        if line == "":
            continue
        sp = line.find(" ")
        if sp < 0:
            continue
        mapping[_trim(line[:sp])] = _trim(line[sp + 1:])
    return mapping


def _slug_from_url(url: str) -> str:
    found = re.search(r"[:/]([^/]*/[^/]*)\.git$", url)
    return found.group(1) if found else ""


def _api_pr_map(repo: Path, rev_range: str) -> dict[str, str]:
    """sha -> PR number. Decoration only: every failure degrades to an empty map,
    so a missing token or an offline host can never block a release."""
    slug = os.environ.get("GITHUB_REPOSITORY", "")
    if not slug:
        slug = _slug_from_url(_git_text(repo, "config", "--get", "remote.origin.url", fatal=False))
    if not slug or shutil.which("gh") is None:
        return {}
    mapping: dict[str, str] = {}
    for short in _git_text(repo, "log", "--no-merges", "--format=%h", rev_range).split("\n"):
        short = short.strip()
        if not short:
            continue
        try:
            proc = subprocess.run(
                ["gh", "api", f"repos/{slug}/commits/{short}/pulls", "--jq", ".[0].number // empty"],
                cwd=str(repo),
                stdout=subprocess.PIPE,
                stderr=None,
                text=True,
                encoding="utf-8",
                errors="replace",
                check=False,
            )
        except OSError:
            continue
        number = (proc.stdout or "").strip()
        if proc.returncode == 0 and number:
            mapping[short] = number
    return mapping


def _records(repo: Path, rev_range: str) -> list[tuple[str, str, str]]:
    """(short sha, full sha, subject) in `git log` order: newest commit first."""
    rows: list[tuple[str, str, str]] = []
    out = _git_text(repo, "log", "--no-merges", f"--format=%h{UNIT}%H{UNIT}%s", rev_range)
    for line in out.split("\n"):
        fields = line.split(UNIT)
        if len(fields) < RECORD_FIELDS:
            continue
        rows.append((fields[0], fields[1], fields[2]))
    return rows


def _breaking_shas(repo: Path, rev_range: str) -> set[str]:
    out = _git_text(repo, "log", "--no-merges", "--format=%H", BREAKING_GREP, rev_range)
    return {line.strip() for line in out.split("\n") if line.strip()}


def _buckets(
    rows: list[tuple[str, str, str]], breaking: set[str], pr_map: dict[str, str], types: _Types
) -> dict[str, list[str]]:
    """The ten display rules, in the original order; the key regex dropouts and
    the `(`-without-`)` drop are load-bearing (they keep junk out of public notes)."""
    buckets: dict[str, list[str]] = {}
    for short, full, subject in rows:
        if subject.startswith(MERGE_PREFIXES):
            continue
        colon = subject.find(":")
        if colon < 0:
            continue
        pre = subject[:colon]
        msg = _trim(subject[colon + 1:])
        if msg == "":
            continue
        bang = pre.endswith("!")
        if bang:
            pre = pre[:-1]
        scope = ""
        paren = pre.find("(")
        if paren >= 0:
            if not pre.endswith(")"):
                continue
            commit_type = pre[:paren]
            scope = _trim(pre[paren + 1 : len(pre) - 1]).lower()
        else:
            commit_type = pre
        if TYPE_RE.fullmatch(commit_type) is None:
            continue
        lowered = commit_type.lower()
        is_breaking = bang or full in breaking or lowered == "breaking"
        lead = f"**{scope}** " if scope else ""
        if is_breaking:
            # A breaking entry echoes its type word back, except a bare `breaking:`.
            line = f"- {lead}{msg}" if lowered == "breaking" else f"- {lead}{lowered}: {msg}"
        elif scope:
            line = f"- **{scope}**: {msg}"
        else:
            line = f"- {msg}"
        line += f" ({short})"
        if short in pr_map and PR_TAIL_RE.search(msg) is None:
            line += f" #{pr_map[short]}"
        if is_breaking:
            bucket = BREAKING_BUCKET
        elif lowered in types.hidden:
            continue
        elif lowered in types.emoji:
            bucket = lowered
        else:
            bucket = OTHER_BUCKET
        buckets.setdefault(bucket, []).append(line)
    return buckets


def _body(version: str, date: str, buckets: dict[str, list[str]], types: _Types) -> str:
    """Heading, then one blank line before each non-empty section, in conf order."""
    sections: list[tuple[str, str, str]] = []
    if buckets.get(BREAKING_BUCKET):
        if "breaking" in types.emoji:
            sections.append((BREAKING_BUCKET, types.emoji["breaking"], types.label["breaking"]))
        else:
            sections.append((BREAKING_BUCKET, BREAKING_EMOJI, BREAKING_LABEL))
    for key in types.order:
        if key == "breaking" or not buckets.get(key):
            continue
        sections.append((key, types.emoji[key], types.label[key]))
    if buckets.get(OTHER_BUCKET):
        sections.append((OTHER_BUCKET, OTHER_EMOJI, OTHER_LABEL))

    lines = [f"## {version} {EM_DASH} {date}"]
    if not sections:
        lines += ["", EMPTY_NOTICE]
        return "\n".join(lines) + "\n"
    for bucket, emoji, label in sections:
        lines += ["", f"### {emoji} {label}"] + buckets[bucket]
    return "\n".join(lines) + "\n"


def _render(
    repo: Path,
    prev: str,
    head: str,
    version: str,
    conf: Path | None,
    pr_map: dict[str, str],
    date: str,
) -> str:
    types = _types_for(conf)
    if not version.startswith("v"):
        version = f"v{version}"
    rows: list[tuple[str, str, str]] = []
    breaking: set[str] = set()
    if prev:
        rev_range = f"{prev}..{head}"
        rows = _records(repo, rev_range)
        breaking = _breaking_shas(repo, rev_range)
    return _body(version, date, _buckets(rows, breaking, pr_map, types), types)


def _baseline_text(version: str, date: str, head: str) -> str:
    """No ancestor tag: replay no history instead of inventing a range."""
    return f"## v{version} {EM_DASH} {date}\n\n{BASELINE_LINE.format(head=head)}\n"


def _derive(repo: Path) -> tuple[int, str, dict[str, str]]:
    """`next-version --format env` in-process (C3), no shell and no eval.

    Returns (exit code, raw env text, parsed lines). The child resolves the
    repository from the process CWD, so run it from `repo` like the shell's
    `cd "$ROOT"` did before the eval.
    """
    from . import version  # lazy: only the derivation fallback needs this module

    buffer = io.StringIO()
    cwd = os.getcwd()
    try:
        os.chdir(repo)
        with contextlib.redirect_stdout(buffer):
            code = version.next_version(["--format", "env"])
    finally:
        os.chdir(cwd)
    text = buffer.getvalue()
    parsed: dict[str, str] = {}
    for line in text.split("\n"):
        if "=" not in line:
            continue
        key, _, value = line.partition("=")
        parsed[key] = value
    return code, text, parsed


def _notes(argv: list[str]) -> tuple[int, str]:
    """Everything `release-notes` does except writing stdout."""
    version = os.environ.get("KIRIVERS_VERSION", "")
    prev = os.environ.get("KIRIVERS_PREV", "")
    head = "HEAD"
    config = ""
    pr_map_path = ""
    fixed_date = ""
    explicit_range = False
    checked = False

    index = 0
    while index < len(argv):
        arg = argv[index]
        if arg in ("-h", "--help"):
            return 0, USAGE + "\n"
        if arg == "--self-check":
            code = _self_check()
            if code:
                return code, ""
            checked = True
            index += 1
            continue
        if arg in _VALUE_FLAGS:
            # The shell took the value from the next arg unconditionally, so a
            # trailing flag means an empty value and the derivation fallback.
            value = argv[index + 1] if index + 1 < len(argv) else ""
            if arg == "--version":
                version = value
            elif arg == "--config":
                config = value
            elif arg == "--pr-map":
                pr_map_path = value
            else:
                fixed_date = value
            index += 2
            continue
        if arg.startswith("-"):
            common.log(f"release-notes: unknown option {arg}")
            return common.USAGE_EXIT, ""
        if not explicit_range:
            prev = arg
            explicit_range = True
        else:
            head = arg
        index += 1

    if checked:
        return 0, ""

    conf = Path(config) if config else common.ROOT / DEFAULT_CONF_NAME
    pr_map: dict[str, str] = {}
    if pr_map_path:
        # Existence is checked against the caller's CWD, like the shell did before
        # it moved to the repository root.
        resolved = Path(pr_map_path).resolve()
        if not resolved.is_file():
            common.log(f"release-notes: --pr-map file not found: {pr_map_path}")
            return common.USAGE_EXIT, ""
        pr_map = _read_pr_map(resolved)

    try:
        if not version or (not explicit_range and not prev):
            code, _, env = _derive(common.ROOT)
            if code:
                return code, ""
            if not version:
                version = env.get("KIRIVERS_VERSION", "")
            if not explicit_range and not prev:
                prev = env.get("KIRIVERS_PREV", "")
        version = version or "0.0.0"
        date_out = fixed_date or datetime.now(timezone.utc).strftime("%Y-%m-%d")
        if not prev:
            # An explicit empty prev skips the derivation above, which is how the
            # first release gets this text instead of a replay of all history.
            return 0, _baseline_text(version, date_out, head)
        if not pr_map_path:
            pr_map = _api_pr_map(common.ROOT, f"{prev}..{head}")
        return 0, _render(common.ROOT, prev, head, version, conf, pr_map, date_out)
    except _Abort as abort:
        if abort.message:
            common.log(abort.message)
        return abort.code, ""


def _pin_utf8() -> None:
    """Pin both streams to UTF-8 before anything is emitted.

    The notes body is UTF-8 by contract (C8), and a Windows host locale such as
    cp936 raises on the section emoji instead; common.force_lf only pins newlines.
    """
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, "reconfigure"):
            stream.reconfigure(encoding="utf-8", newline="\n")


def release_notes(argv: list[str]) -> int:
    _pin_utf8()
    code, body = _notes(argv)
    if body:
        sys.stdout.write(body)
    return code


# ------------------------------------------------------------------ changelog

PROLOGUE = (
    "# Changelog\n\n"
    "<!-- Sections are appended by dev/build/kirivers.py changelog during a release; "
    "keep edits above this marker. -->\n\n"
)


def _split_records(text: str) -> list[str]:
    """Text as awk saw it: one entry per line, no phantom last record."""
    if text == "":
        return []
    lines = text.split("\n")
    if text.endswith("\n"):
        lines.pop()
    return lines


def _read_records(path: Path) -> list[str]:
    return _split_records(path.read_text(encoding="utf-8", errors="replace"))


def _splice(records: list[str], notes_lines: list[str], version: str) -> list[str]:
    """Insert the notes above the first `## ` heading, replacing a same-version
    block (first occurrence only) and keeping any human preamble."""
    head = re.compile(r"## " + re.escape(version) + r"([ \t]|$)")
    total = len(records)
    first = oldstart = oldend = 0
    for number, line in enumerate(records, 1):
        if not line.startswith("## "):
            continue
        if first == 0:
            first = number
        if oldstart == 0:
            if head.match(line):
                oldstart = number
        elif oldend == 0:
            oldend = number
    if oldstart and not oldend:
        oldend = total + 1
    if not first:
        first = total + 1

    out = records[: first - 1]
    if first > 1 and records[first - 2] != "":
        out.append("")
    out += notes_lines
    rest = oldend if oldstart else first
    if rest <= total:
        out.append("")
        out += records[rest - 1 :]
    return out


def changelog(argv: list[str]) -> int:
    """Splice generated notes into the head of CHANGELOG.md; stdout stays empty."""
    _pin_utf8()
    target = Path(os.environ.get("KIRIVERS_CHANGELOG_FILE") or (common.ROOT / "CHANGELOG.md"))
    code, notes = _notes(argv)
    if code:
        return code
    if not notes:
        common.log("changelog: release-notes produced no output")
        return common.FAIL_EXIT
    first_line = notes.split("\n", 1)[0]
    found = HEADING_VERSION_RE.match(first_line)
    if found is None:
        common.log(f"changelog: notes heading has no version: {first_line}")
        return common.FAIL_EXIT
    version = found.group(1)

    try:
        if not target.is_file():
            common.write_lf(target, PROLOGUE + notes)
            common.log(f"changelog: created {target} with {version}")
            return 0
        merged = _splice(_read_records(target), _split_records(notes), version)
        common.write_lf(target, "\n".join(merged) + "\n")
    except OSError as error:
        common.log(f"changelog: cannot write {target}: {error}")
        return common.FAIL_EXIT
    common.log(f"changelog: updated {target} with {version}")
    return 0


# ------------------------------------------------------------------ self check

_FIXTURES = (
    "chore: seed",
    "feat(feed): add webhook delivery",
    "fix(client): 修正空响应",
    "feat!: drop legacy v1 endpoint",
    "perf(DB): cut allocation",
    "docs: rewrite install page",
    "refactor: extract query builder",
    "deps: bump object store client",
    "plain subject without a type",
)

# Breaking entry first, then the conf rows in file order, then Other.
_FIXTURE_ORDER = (
    f"### {BREAKING_EMOJI} {BREAKING_LABEL}",
    "### ✨ Features",
    "### 🐛 Bug Fixes",
    "### ⚡ Performance",
    "### ♻️ Refactoring",
    f"### {OTHER_EMOJI} {OTHER_LABEL}",
)

_BAD_CONFS = (
    ("type key must start with a letter", "9feat=✨=Features\n"),
    ("an uppercase type key must exit non-zero", "Feat=✨=Features\n"),
    ("an empty value must exit non-zero", "feat=\n"),
    ("a duplicated type key must exit non-zero", "feat=✨=Features\nfeat=🎉=More Features\n"),
)


def _must(argv: list[str]) -> None:
    """Run a fixture-side git command; its stdout is already diverted to stderr."""
    if common.run(argv):
        raise _Abort(common.FAIL_EXIT, f"FAIL: {' '.join(argv[:3])} failed")


def _self_check() -> int:
    """Offline fixture assertions; stdout stays empty, status goes to stderr.

    The shell version had to pin LC_ALL=C so grep matched non-ASCII bytes; the
    comparisons below are in-memory on decoded text, so they are locale-free.
    """
    failed = 0
    try:
        with tempfile.TemporaryDirectory(prefix="kirivers-notes-") as raw:
            failed = _check_fixture(Path(raw))
    except _Abort as abort:
        common.log(abort.message or f"FAIL: fixture render aborted ({abort.code})")
        failed += 1
    except OSError as error:
        common.log(f"FAIL: fixture setup failed: {error}")
        failed += 1
    if failed:
        common.log("release-notes: self-check FAILED")
        return common.FAIL_EXIT
    common.log("release-notes: self-check ok")
    return 0


def _check_fixture(tmp: Path) -> int:
    failed = 0
    seq = 0
    repo = tmp / "repo"
    _must(["git", "init", "-q", str(repo)])
    _must(["git", "-C", str(repo), "config", "user.name", "selfcheck"])
    _must(["git", "-C", str(repo), "config", "user.email", "selfcheck@example.com"])

    def commit(subject: str, body_file: Path | None = None) -> None:
        nonlocal seq
        seq += 1
        argv = [
            "git",
            "-C",
            str(repo),
            # core.autocrlf on a maintainer checkout would otherwise spam warnings.
            "-c",
            "core.autocrlf=false",
            "-c",
            "core.safecrlf=false",
        ]
        common.write_lf(repo / f"f{seq}.txt", f"payload-{seq}\n")
        _must(argv + ["add", "-A"])
        if body_file is None:
            _must(argv + ["commit", "-q", "-m", subject])
            return
        message = tmp / ".msg"
        common.write_lf(message, f"{subject}\n\n{body_file.read_text(encoding='utf-8')}")
        _must(argv + ["commit", "-q", "--cleanup=whitespace", "-F", str(message)])
        message.unlink(missing_ok=True)

    # Oldest first; git log lists newest first, which is the entry order.
    for subject in _FIXTURES:
        commit(subject)
    breaking_body = tmp / "body"
    common.write_lf(
        breaking_body,
        "Refactors internals.\nBREAKING CHANGE: removes the old rotation API\n",
    )
    commit("feat(admin): rewrite token rotation", breaking_body)
    commit("fix(delta): reject truncated header")
    # The tag sits on `chore: seed` so every row above is inside the range.
    _must(["git", "-C", str(repo), "tag", "-a", "v0.1.0", "-m", "v0.1.0", "HEAD~10"])

    def short_of(needle: str) -> str:
        for line in _git_text(repo, "log", "--format=%h %s", "v0.1.0..HEAD").split("\n"):
            if needle in line:
                return line.split(" ")[0]
        return ""

    pr_map_file = tmp / "prmap"
    common.write_lf(
        pr_map_file, f"{short_of('fix(delta)')} 29\n{short_of('feat(admin)')} 31\n"
    )
    pr_map = _read_pr_map(pr_map_file)
    shipped = common.ROOT / DEFAULT_CONF_NAME
    date = "2026-01-02"
    # `feat!:` is in range, so the derived version is a major bump.
    notes = _render(repo, "v0.1.0", "HEAD", "1.0.0", shipped, pr_map, date)

    def expect(needle: str) -> None:
        nonlocal failed
        if needle not in notes:
            common.log(f"FAIL: expected line not found: {needle}")
            common.log(notes.rstrip("\n"))
            failed += 1

    def expect_absent(needle: str) -> None:
        nonlocal failed
        if needle in notes:
            common.log(f"FAIL: line must not appear: {needle}")
            common.log(notes.rstrip("\n"))
            failed += 1

    def expect_before(earlier: str, later: str, haystack: str = "") -> None:
        nonlocal failed
        text = haystack or notes
        first, second = text.find(earlier), text.find(later)
        if first < 0 or second < 0 or first >= second:
            common.log(f"FAIL: section order: '{earlier}' must precede '{later}'")
            failed += 1

    def render(
        conf: Path | None,
        prev: str = "v0.1.0",
        head: str = "HEAD",
        mapping: dict[str, str] | None = None,
    ) -> str | None:
        """None marks the expected failure of an invalid type table."""
        try:
            return _render(
                repo, prev, head, "1.0.0", conf, {} if mapping is None else mapping, date
            )
        except _ConfError:
            return None

    expect(f"## v1.0.0 {EM_DASH} {date}")
    # Breaking entries carry the type word back.
    expect("- **admin** feat: rewrite token rotation")
    expect(" #31")
    expect("- feat: drop legacy v1 endpoint")
    # Scope is lowercased and used as a per-entry prefix.
    expect("- **db**: cut allocation")
    expect("- **feed**: add webhook delivery")
    expect("- **delta**: reject truncated header")
    expect(" #29")
    expect("- **client**: 修正空响应")
    expect("- extract query builder")
    # An unregistered type falls back to Other.
    expect("- bump object store client")
    expect(f"### {OTHER_EMOJI} {OTHER_LABEL}")
    # Hidden types and non-conventional subjects stay out.
    expect_absent("rewrite install page")
    expect_absent("seed")
    expect_absent("plain subject without a type")
    expect_absent("chore")
    expect_absent("docs")
    for earlier, later in zip(_FIXTURE_ORDER, _FIXTURE_ORDER[1:]):
        expect_before(earlier, later)

    # Determinism: two runs are byte-identical.
    again = render(shipped, mapping=pr_map)
    if again != notes:
        common.log("FAIL: output is not deterministic")
        failed += 1

    # The embedded default table must parse identically to the shipped file.
    default = render(None, mapping=pr_map)
    if default != notes:
        common.log(f"FAIL: embedded default table diverges from {shipped}")
        common.log((default or "").rstrip("\n"))
        failed += 1

    # AC14: hiding a type changes the display only, never the version rank.
    code_before, before, env_before = _derive(repo)
    hidden_conf = tmp / "hidden.conf"
    shipped_text = shipped.read_text(encoding="utf-8", errors="replace")
    common.write_lf(
        hidden_conf,
        "\n".join(
            ("# " + line) if line.startswith("fix=") else line
            for line in shipped_text.split("\n")
        ),
    )
    hidden = render(hidden_conf, mapping=pr_map)
    if hidden is None or "### \U0001F41B Bug Fixes" in hidden:
        common.log("FAIL: commenting out fix= must drop the Bug Fixes section")
        failed += 1
    if hidden is None or "cut allocation" not in hidden:
        common.log("FAIL: hiding fix= must not hide other sections")
        failed += 1
    code_after, after, _env_after = _derive(repo)
    if code_before or code_after:
        common.log("FAIL: next-version must succeed in the fixture repo")
        failed += 1
    if before != after:
        common.log("FAIL: next-version output changed with the display config")
        common.log(f"before:\n{before}after:\n{after}")
        failed += 1
    if env_before.get("KIRIVERS_VERSION") != "1.0.0":
        common.log("FAIL: fixture range must bump major to 1.0.0")
        common.log(before.rstrip("\n"))
        failed += 1

    # Reordering the conf must reorder the sections.
    reordered_conf = tmp / "reordered.conf"
    common.write_lf(reordered_conf, "fix=🐛=Bug Fixes\nfeat=✨=Features\n")
    reordered = render(reordered_conf, mapping=pr_map) or ""
    expect_before("### \U0001F41B Bug Fixes", "### ✨ Features", reordered)
    if reordered == "":
        failed += 1

    # A range without displayable commits says so instead of emitting empties.
    commit("chore(ci): tune runner cache")
    empty = render(shipped, prev="HEAD~1", head="HEAD") or ""
    if EMPTY_NOTICE not in empty:
        common.log("FAIL: empty range must print the explicit notice")
        common.log(empty.rstrip("\n"))
        failed += 1
    if "### " in empty:
        common.log("FAIL: empty range must not emit sections")
        failed += 1

    # Baseline (no ancestor tag) replays no history.
    baseline = _baseline_text("0.1.0", date, "HEAD")
    if "Initial release baseline" not in baseline:
        common.log("FAIL: baseline text missing")
        common.log(baseline.rstrip("\n"))
        failed += 1
    if "### " in baseline:
        common.log("FAIL: baseline must not emit sections")
        failed += 1

    # Invalid config rows fail loudly instead of being ignored.
    for number, (reason, row) in enumerate(_BAD_CONFS):
        bad_conf = tmp / f"bad{number}.conf"
        common.write_lf(bad_conf, row)
        if render(bad_conf) is not None:
            common.log(f"FAIL: {reason}")
            failed += 1

    return failed
