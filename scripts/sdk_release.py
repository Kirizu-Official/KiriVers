#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""SDK release planner: Conventional Commits bump, prefixed tags, changelog.

Runs on each language package root, driven by that branch's own GitHub Actions
workflow: a merged pull request bumps the manifest, tags ``sdk-<lang>-v<x.y.z>``,
publishes the GitHub Release on the branch and (for registry languages) uploads.

Stdlib only. Commands: plan | apply | commitlint | identity | self-check

The changelog section format is shared with the server pipeline
(dev/build/release-notes.sh on main): sections come from the commit type, the
type -> emoji/label/order mapping lives in scripts/changelog-types.conf, and
entries are prefixed with the commit scope. That table is display-only: the
version bump below never reads it.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

SEMVER = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")
SUBJECT = re.compile(
    r"^(?P<type>[a-zA-Z][a-zA-Z0-9-]*)"
    r"(?:\((?P<scope>[^()]*)\))?(?P<bang>!?):\s+(?P<subject>.+)$"
)
# Bump ranks. Keeping this table free of any changelog styling is the invariant
# that lets maintainers comment a type out of scripts/changelog-types.conf
# without silently disabling releases.
RELEASE_TYPES = {"feat": "minor", "fix": "patch", "perf": "patch", "revert": "patch"}
RANK = {"none": 0, "patch": 1, "minor": 2, "major": 3}

BREAKING_RE = re.compile(r"^BREAKING[ -]CHANGE:", re.MULTILINE)
CONVENTIONAL_TYPES = {
    "feat",
    "fix",
    "perf",
    "revert",
    "docs",
    "refactor",
    "test",
    "style",
    "chore",
    "ci",
    "build",
}

# Default type -> (emoji, label). The order of this tuple *is* the section order
# when scripts/changelog-types.conf is missing; commented entries hide a type.
DEFAULT_TYPE_TABLE: list[tuple[str, str, str, bool]] = [
    ("breaking", "\U0001f4a5", "Breaking Changes", False),
    ("feat", "✨", "Features", False),
    ("fix", "\U0001f41b", "Bug Fixes", False),
    ("perf", "⚡", "Performance", False),
    ("revert", "⏪", "Reverts", False),
    ("refactor", "♻️", "Refactoring", False),
    ("docs", "\U0001f4dd", "Documentation", True),
    ("test", "✅", "Tests", True),
    ("style", "💄", "Style", True),
    ("chore", "\U0001f527", "Chores", True),
    ("ci", "\U0001f477", "CI", True),
    ("build", "\U0001f4e6", "Build", True),
]
OTHER_EMOJI = "\U0001f516"
OTHER_LABEL = "Other"

# SDK_LANG → tag prefix, manifest kind, and the repository a registry publish is
# allowed to come from (D12: go / php / swift publish from their own repo).
LANGS: dict[str, dict[str, Any]] = {
    "python": {
        "prefix": "sdk-python-v",
        "kind": "pyproject",
        "path": "pyproject.toml",
        "publish_repo": None,
    },
    "java": {
        "prefix": "sdk-java-v",
        "kind": "pom",
        "path": "pom.xml",
        "publish_repo": None,
        "artifact": "kirivers-client",
    },
    "kotlin": {
        "prefix": "sdk-kotlin-v",
        "kind": "gradle",
        "path": "build.gradle.kts",
        "publish_repo": None,
    },
    "csharp": {
        "prefix": "sdk-csharp-v",
        "kind": "csproj",
        "path": "Kirizu.KiriVers.Client.csproj",
        "publish_repo": None,
    },
    "typescript": {
        "prefix": "sdk-typescript-v",
        "kind": "packagejson",
        "path": "package.json",
        "publish_repo": None,
    },
    "rust": {
        "prefix": "sdk-rust-v",
        "kind": "cargo",
        "path": "Cargo.toml",
        "publish_repo": None,
    },
    "dart": {
        "prefix": "sdk-dart-v",
        "kind": "pubspec",
        "path": "pubspec.yaml",
        "publish_repo": None,
    },
    "c": {
        "prefix": "sdk-c-v",
        "kind": "cmake",
        "path": "CMakeLists.txt",
        "publish_repo": None,
    },
    "cpp": {
        "prefix": "sdk-cpp-v",
        "kind": "cmake",
        "path": "CMakeLists.txt",
        "publish_repo": None,
    },
    "go": {
        "prefix": "v",
        "kind": "tag-only",
        "path": None,
        "publish_repo": "Kirizu-Official/KiriVers-SDK-Go",
    },
    "php": {
        "prefix": "v",
        "kind": "tag-only",
        "path": None,
        "publish_repo": "Kirizu-Official/KiriVers-SDK-PHP",
    },
    "swift": {
        "prefix": "v",
        "kind": "tag-only",
        "path": None,
        "publish_repo": "Kirizu-Official/KiriVers-SDK-Swift",
    },
}

CMAKE_VERSION_ARG = re.compile(r"(?i)(VERSION\s+)(\d+\.\d+\.\d+)")


def cmake_blocks(text: str) -> list[tuple[int, int, int]]:
    """Yield (start, end, inner_end) for each top-level ``project(...)`` call.

    start points at ``project``, end is just past the closing paren, inner_end is
    the index of that closing paren. Nesting is tracked because the argument list
    can hold generator expressions.
    """
    out: list[tuple[int, int, int]] = []
    lowered = text.lower()
    pos = 0
    while True:
        i = lowered.find("project", pos)
        if i < 0:
            return out
        if i and (text[i - 1].isalnum() or text[i - 1] == "_"):
            pos = i + 7
            continue
        j = i + 7
        while j < len(text) and text[j] in " \t\r\n":
            j += 1
        if j >= len(text) or text[j] != "(":
            pos = i + 7
            continue
        depth = 0
        for k in range(j, len(text)):
            if text[k] == "(":
                depth += 1
            elif text[k] == ")":
                depth -= 1
                if depth == 0:
                    out.append((i, k + 1, k))
                    pos = k + 1
                    break
        else:
            return out


def die(msg: str, code: int = 1) -> None:
    print(msg, file=sys.stderr)
    raise SystemExit(code)


def git(*args: str, check: bool = True) -> str:
    r = subprocess.run(
        ["git", *args],
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
    )
    if check and r.returncode != 0:
        die(r.stderr.strip() or f"git {' '.join(args)} failed ({r.returncode})")
    return r.stdout


def cfg() -> dict[str, Any]:
    lang = os.environ.get("SDK_LANG", "").strip()
    if lang not in LANGS:
        die(f"SDK_LANG must be one of: {', '.join(sorted(LANGS))}")
    return {"lang": lang, **LANGS[lang]}


def parse_semver(s: str) -> tuple[int, int, int]:
    m = SEMVER.match(s)
    if not m:
        die(f"not a x.y.z version: {s!r}")
    return int(m.group(1)), int(m.group(2)), int(m.group(3))


def fmt_semver(v: tuple[int, int, int]) -> str:
    return f"{v[0]}.{v[1]}.{v[2]}"


def bump_semver(v: tuple[int, int, int], kind: str) -> tuple[int, int, int]:
    major, minor, patch = v
    if kind == "major":
        return (major + 1, 0, 0)
    if kind == "minor":
        return (major, minor + 1, 0)
    if kind == "patch":
        return (major, minor, patch + 1)
    die(f"unknown bump {kind}")
    raise AssertionError


def publish_flag_enabled() -> bool:
    raw = os.environ.get("SDK_PUBLISH", "true").strip().lower()
    return raw not in {"0", "false", "no", "off"}


def identity_allows_publish(meta: dict[str, Any]) -> bool:
    want = meta.get("publish_repo")
    if not want:
        return True
    return os.environ.get("GITHUB_REPOSITORY", "").strip() == str(want)


def previous_release_tag(prefix: str) -> str | None:
    listed = git("tag", "-l", f"{prefix}*")
    found: list[tuple[tuple[int, int, int], str]] = []
    for line in listed.splitlines():
        tag = line.strip()
        if not tag.startswith(prefix):
            continue
        ver = tag[len(prefix) :]
        if not SEMVER.match(ver):
            continue
        anc = subprocess.run(
            ["git", "merge-base", "--is-ancestor", tag, "HEAD"],
            capture_output=True,
        )
        if anc.returncode == 0:
            found.append((parse_semver(ver), tag))
    if not found:
        return None
    found.sort()
    return found[-1][1]


# --------------------------------------------------------------- changelog types


class TypeTable:
    """Ordered ``type -> emoji/label`` map; hidden types stay out of the log."""

    def __init__(self, entries: list[tuple[str, str, str]], hidden: set[str]) -> None:
        self.order = [e[0] for e in entries]
        self.emoji = {e[0]: e[1] for e in entries}
        self.label = {e[0]: e[2] for e in entries}
        self.hidden = hidden

    def section(self, type_name: str) -> tuple[str, str]:
        return self.emoji.get(type_name, OTHER_EMOJI), self.label.get(type_name, OTHER_LABEL)

    def is_hidden(self, type_name: str) -> bool:
        return type_name in self.hidden


KEY_RE = re.compile(r"^[a-z][a-z0-9-]*$")
SPLIT_RE = re.compile(r"[ \t=]")


def parse_type_lines(lines: list[str], origin: str) -> TypeTable:
    """Parse `type=emoji[=label]`, where a leading `#` hides the type."""
    entries: list[tuple[str, str, str]] = []
    hidden: set[str] = set()
    seen: dict[str, int] = {}
    for lineno, raw in enumerate(lines, 1):
        line = raw.rstrip("\r")
        if not line.strip():
            continue
        commented = line.startswith("#")
        body = line[1:] if commented else line
        if commented:
            probe = body.lstrip()
            # Only `# type=...` hides a type; any other comment line is prose.
            if not re.match(r"^[a-z][a-z0-9-]*[ \t]*=", probe):
                continue
        if "=" not in body:
            die(f"{origin}:{lineno}: expected type=emoji[=label], got: {raw!r}")
        key, _, value = body.partition("=")
        key = key.strip()
        value = value.strip()
        if not KEY_RE.match(key):
            die(f"{origin}:{lineno}: invalid type key {key!r}")
        if not value:
            die(f"{origin}:{lineno}: missing emoji for {key!r}")
        m = SPLIT_RE.search(value)
        if m is None:
            emoji, label = value, key
        else:
            emoji = value[: m.start()].strip()
            label = value[m.end() :].strip() or key
        if not emoji or SPLIT_RE.search(emoji):
            die(f"{origin}:{lineno}: emoji must be a single token for {key!r}")
        if commented:
            if key in seen:
                die(f"{origin}:{lineno}: {key} is both active and commented out")
            hidden.add(key)
            continue
        if key in seen:
            die(f"{origin}:{lineno}: duplicate type {key!r}")
        if key in hidden:
            die(f"{origin}:{lineno}: {key} is both active and commented out")
        seen[key] = lineno
        entries.append((key, emoji, label))
    if not any(e[0] == "breaking" for e in entries):
        entries.insert(0, ("breaking", "\U0001f4a5", "Breaking Changes"))
    return TypeTable(entries, hidden)


def default_type_table() -> TypeTable:
    lines = []
    for name, emoji, label, hidden in DEFAULT_TYPE_TABLE:
        lines.append(f"{'#' if hidden else ''}{name}={emoji}={label}")
    return parse_type_lines(lines, "<embedded default>")


def type_table() -> TypeTable:
    path = Path(__file__).resolve().parent / "changelog-types.conf"
    if not path.is_file():
        return default_type_table()
    return parse_type_lines(
        path.read_text(encoding="utf-8").splitlines(), str(path.name)
    )


# --------------------------------------------------------------- manifest kinds


def cmake_read(text: str) -> str:
    for start, _end, inner_end in cmake_blocks(text):
        v = CMAKE_VERSION_ARG.search(text[start:inner_end])
        if v:
            return v.group(2)
    return ""


def cmake_write(text: str, new: str) -> tuple[str, int]:
    for start, end, inner_end in cmake_blocks(text):
        inner = text[start:end]
        v = CMAKE_VERSION_ARG.search(text[start:inner_end])
        if not v:
            continue
        head = inner[: v.start()]
        tail = inner[v.end() :]
        return text[:start] + head + v.group(1) + new + tail + text[end:], 1
    return text, 0


def read_manifest_version(meta: dict[str, Any]) -> str:
    kind = meta["kind"]
    if kind == "tag-only":
        return "0.1.0"
    path = Path(meta["path"])
    if not path.is_file():
        die(f"version file missing: {path}")
    text = path.read_text(encoding="utf-8")
    if kind == "pyproject":
        m = re.search(r'(?m)^version\s*=\s*"([^"]+)"', text)
    elif kind == "packagejson":
        m = re.search(r'"version"\s*:\s*"([^"]+)"', text)
    elif kind == "cargo":
        m = re.search(
            r"(?ms)^\[package\].*?^version\s*=\s*\"([^\"]+)\"",
            text,
        )
    elif kind == "pubspec":
        m = re.search(r"(?m)^version:\s*(\S+)", text)
    elif kind == "gradle":
        m = re.search(r'(?m)^version\s*=\s*"([^"]+)"', text)
    elif kind == "csproj":
        m = re.search(r"<Version>([^<]+)</Version>", text)
    elif kind == "cmake":
        ver = cmake_read(text)
        return ver if ver else die(f"could not read version from {path}")
    elif kind == "pom":
        art = re.escape(str(meta.get("artifact") or "kirivers-client"))
        m = re.search(
            rf"<artifactId>{art}</artifactId>\s*<version>([^<]+)</version>",
            text,
            re.DOTALL,
        )
    else:
        die(f"unknown kind {kind}")
    if not m:
        die(f"could not read version from {path}")
    return m.group(1).strip()


def write_manifest_version(meta: dict[str, Any], new: str) -> None:
    kind = meta["kind"]
    if kind == "tag-only":
        return
    path = Path(meta["path"])
    text = path.read_text(encoding="utf-8")
    if kind == "pyproject":
        text, n = re.subn(r'(?m)^(version\s*=\s*")[^"]+"', rf"\g<1>{new}\"", text, count=1)
    elif kind == "packagejson":
        text, n = re.subn(r'("version"\s*:\s*")[^"]+"', rf"\g<1>{new}\"", text, count=1)
    elif kind == "cargo":
        lines: list[str] = []
        in_pkg = False
        n = 0
        for line in text.splitlines(keepends=True):
            s = line.strip()
            if s.startswith("[") and s.endswith("]"):
                in_pkg = s == "[package]"
            if in_pkg and n == 0 and re.match(r"version\s*=", line):
                line = re.sub(r'(version\s*=\s*")[^"]+"', rf"\g<1>{new}\"", line)
                n = 1
            lines.append(line)
        text = "".join(lines)
    elif kind == "pubspec":
        text, n = re.subn(r"(?m)^(version:\s*)\S+", rf"\g<1>{new}", text, count=1)
    elif kind == "gradle":
        text, n = re.subn(r'(?m)^(version\s*=\s*")[^"]+"', rf"\g<1>{new}\"", text, count=1)
    elif kind == "csproj":
        text, n = re.subn(r"(<Version>)[^<]+(</Version>)", rf"\g<1>{new}\2", text, count=1)
    elif kind == "cmake":
        text, n = cmake_write(text, new)
    elif kind == "pom":
        art = re.escape(str(meta.get("artifact") or "kirivers-client"))
        text, n = re.subn(
            rf"(<artifactId>{art}</artifactId>\s*<version>)[^<]+",
            rf"\g<1>{new}",
            text,
            count=1,
            flags=re.DOTALL,
        )
    else:
        die(f"unknown kind {kind}")
        return
    if n != 1:
        die(f"failed to write version to {path} (replacements={n})")
    path.write_text(text, encoding="utf-8")


# ---------------------------------------------------------------- commits


def classify_message(subject: str, body: str) -> str:
    """Return none|patch|minor|major for one commit. Never consults the type table."""
    subj = subject.strip()
    m = SUBJECT.match(subj)
    if not m:
        return "none"
    breaking = bool(m.group("bang")) or bool(BREAKING_RE.search(body or ""))
    if breaking:
        return "major"
    return RELEASE_TYPES.get(m.group("type").lower(), "none")


class Commit:
    def __init__(self, short: str, full: str, subject: str, body: str) -> None:
        self.short = short
        self.full = full
        self.subject = subject
        self.body = body

    @property
    def match(self) -> re.Match[str] | None:
        return SUBJECT.match(self.subject.strip())

    @property
    def bump(self) -> str:
        return classify_message(self.subject, self.body)


def collect_commits(since_tag: str | None) -> list[Commit]:
    spec = f"{since_tag}..HEAD" if since_tag else ""
    args = ["log", "--format=%h%x1f%H%x1f%s%x1f%b%x1e", "--no-merges"]
    if spec:
        args.append(spec)
    raw = git(*args)
    out: list[Commit] = []
    for chunk in raw.split("\x1e"):
        chunk = chunk.strip("\n")
        if not chunk.strip():
            continue
        parts = chunk.split("\x1f")
        while len(parts) < 4:
            parts.append("")
        out.append(Commit(parts[0].strip(), parts[1].strip(), parts[2].strip(), parts[3].strip()))
    return out


def highest_bump(commits: list[Commit]) -> str:
    best = "none"
    for c in commits:
        kind = c.bump
        if RANK[kind] > RANK[best]:
            best = kind
    return best


def pr_numbers(commits: list[Commit]) -> dict[str, str]:
    """sha7 -> pull request number. Purely decorative: every failure is ignored."""
    out: dict[str, str] = {}
    if not commits:
        return out
    repo = os.environ.get("GITHUB_REPOSITORY", "").strip()
    fallback = os.environ.get("KIRIVERS_PR_NUMBER", "").strip()
    have_gh = subprocess.run(["bash", "-lc", "command -v gh"], capture_output=True).returncode == 0
    if repo and have_gh:
        for c in commits:
            r = subprocess.run(
                [
                    "gh",
                    "api",
                    f"repos/{repo}/commits/{c.full}/pulls",
                    "--jq",
                    ".[0].number // empty",
                ],
                capture_output=True,
                text=True,
            )
            pr = r.stdout.strip()
            if pr:
                out[c.short] = pr
    if fallback:
        for c in commits:
            out.setdefault(c.short, fallback)
    return out


def release_date() -> str:
    return os.environ.get("KIRIVERS_RELEASE_DATE") or datetime.now(timezone.utc).strftime(
        "%Y-%m-%d"
    )


def entry_line(c: Commit, pr: str | None, breaking: bool, type_name: str) -> str:
    m = c.match
    assert m is not None
    scope = (m.group("scope") or "").strip().lower()
    msg = m.group("subject").strip()
    if breaking:
        head = f"**{scope}** " if scope else ""
        word = "" if type_name == "breaking" else f"{type_name}: "
        line = f"- {head}{word}{msg}"
    elif scope:
        line = f"- **{scope}**: {msg}"
    else:
        line = f"- {msg}"
    line += f" ({c.short})"
    if pr and not re.search(r"\(#[0-9]+\)[ \t]*$", msg):
        line += f" #{pr}"
    return line


def notes_markdown(
    version: str,
    commits: list[Commit],
    baseline: bool,
    table: TypeTable | None = None,
    pr_map: dict[str, str] | None = None,
) -> str:
    heading = f"## v{version} — {release_date()}"
    if baseline:
        return f"{heading}\n\nInitial registry baseline (manifest version, no commit replay).\n"
    table = table or type_table()
    pr_map = pr_map or {}
    buckets: dict[str, list[str]] = {}

    def push(key: str, line: str) -> None:
        buckets.setdefault(key, []).append(line)

    for c in commits:
        m = c.match
        if not m:
            continue
        type_name = m.group("type").lower()
        if c.bump == "major":
            push("__breaking__", entry_line(c, pr_map.get(c.short), True, type_name))
        elif type_name in table.hidden:
            continue
        elif type_name in table.order:
            push(type_name, entry_line(c, pr_map.get(c.short), False, type_name))
        else:
            push("__other__", entry_line(c, pr_map.get(c.short), False, type_name))

    lines = [heading, ""]
    order = ["__breaking__"] + [k for k in table.order if k != "breaking"] + ["__other__"]
    sections = 0
    for key in order:
        items = buckets.get(key)
        if not items:
            continue
        if key == "__breaking__":
            emoji, label = table.section("breaking")
        elif key == "__other__":
            emoji, label = OTHER_EMOJI, OTHER_LABEL
        else:
            emoji, label = table.section(key)
        lines.append(f"### {emoji} {label}")
        lines.extend(items)
        lines.append("")
        sections += 1
    if not sections:
        lines.append("No user-facing Conventional Commits in this range.")
        lines.append("")
    return "\n".join(lines)


def prepend_changelog(version: str, notes: str) -> None:
    """Insert the new section under the `# Changelog` title, creating the file."""
    path = Path("CHANGELOG.md")
    block = notes.rstrip() + "\n\n"
    if not path.is_file():
        path.write_text(f"# Changelog\n\n{block}", encoding="utf-8")
        return
    existing = path.read_text(encoding="utf-8")
    if existing.lstrip().lower().startswith("# changelog"):
        first, _, rest = existing.partition("\n")
        path.write_text(first + "\n\n" + block + rest.lstrip("\n"), encoding="utf-8")
    else:
        path.write_text(block + existing, encoding="utf-8")


def plan_release() -> dict[str, Any]:
    meta = cfg()
    prefix = str(meta["prefix"])
    identity_ok = identity_allows_publish(meta)
    enabled = publish_flag_enabled()
    prev = previous_release_tag(prefix)
    manifest = read_manifest_version(meta)

    result: dict[str, Any] = {
        "lang": meta["lang"],
        "prefix": prefix,
        "previous_tag": prev,
        "identity_ok": identity_ok,
        "publish_enabled": enabled,
        "action": "skip",
        "reason": "",
        "version": manifest,
        "tag": prefix + manifest,
        "bump": None,
        "notes": "",
    }

    if not enabled:
        result["reason"] = "SDK_PUBLISH is false"
        return result
    if not identity_ok:
        result["reason"] = (
            "repository identity is not the D12 publish remote; tests only"
        )
        return result

    if prev is None:
        # D3① baseline: current manifest, do not replay history.
        result["action"] = "baseline"
        result["reason"] = "no matching ancestor tag"
        result["version"] = manifest
        result["tag"] = prefix + manifest
        result["notes"] = notes_markdown(manifest, [], baseline=True)
        return result

    commits = collect_commits(prev)
    bump = highest_bump(commits)
    if bump == "none":
        result["reason"] = "no releasing Conventional Commits since " + prev
        return result
    prev_ver = parse_semver(prev[len(prefix) :])
    new_ver = fmt_semver(bump_semver(prev_ver, bump))
    result["action"] = "bump"
    result["bump"] = bump
    result["version"] = new_ver
    result["tag"] = prefix + new_ver
    result["notes"] = notes_markdown(new_ver, commits, baseline=False, pr_map=pr_numbers(commits))
    return result


def cmd_plan(args: argparse.Namespace) -> int:
    data = plan_release()
    text = json.dumps(data, indent=2, ensure_ascii=False) + "\n"
    sys.stdout.write(text)
    if args.write_notes:
        Path(args.write_notes).write_text(data.get("notes") or "", encoding="utf-8")
    return 0


def cmd_apply(_: argparse.Namespace) -> int:
    data = plan_release()
    json.dump(data, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")
    if data["action"] == "skip":
        return 0
    meta = cfg()
    if data["action"] == "bump":
        write_manifest_version(meta, data["version"])
    if data["action"] in {"bump", "baseline"}:
        prepend_changelog(data["version"], data["notes"])
    Path("sdk-release-notes.md").write_text(data["notes"], encoding="utf-8")
    return 0


def lint_one(label: str, message: str) -> str | None:
    first = message.strip().splitlines()[0] if message.strip() else ""
    if not first:
        return f"{label}: empty message"
    if first.startswith("Merge "):
        return None
    if SUBJECT.match(first):
        return None
    return (
        f"{label}: {first!r} is not Conventional Commits "
        "(feat|fix|perf|revert|docs|refactor|test|style|chore|ci|build)"
    )


def cmd_commitlint(args: argparse.Namespace) -> int:
    errors: list[str] = []
    if args.pr_title:
        err = lint_one("PR title", args.pr_title)
        if err:
            errors.append(err)
    base = args.base
    head = args.head or "HEAD"
    if base:
        raw = git("log", "--format=%s", f"{base}..{head}", "--no-merges", check=False)
        # git log still returns 0 for empty ranges
        for i, line in enumerate(raw.splitlines(), 1):
            err = lint_one(f"commit {i}", line)
            if err:
                errors.append(err)
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    if not args.pr_title and not base:
        die("commitlint needs --pr-title and/or --base")
    print("commitlint ok")
    return 0


def cmd_identity(_: argparse.Namespace) -> int:
    meta = cfg()
    ok = identity_allows_publish(meta) and publish_flag_enabled()
    print(
        json.dumps(
            {
                "identity_ok": identity_allows_publish(meta),
                "publish_enabled": publish_flag_enabled(),
                "allow": ok,
                "github": os.environ.get("GITHUB_REPOSITORY", ""),
                "want": meta.get("publish_repo"),
            },
            indent=2,
        )
    )
    return 0 if ok else 2


# --------------------------------------------------------------- self check


def _commit(short: str, subject: str, body: str = "") -> Commit:
    return Commit(short, short + "0" * 33, subject, body)


def self_check() -> None:
    # Bump ranks (unchanged by any display configuration).
    assert classify_message("feat: x", "") == "minor"
    assert classify_message("fix: x", "") == "patch"
    assert classify_message("feat!: x", "") == "major"
    assert classify_message("feat: x", "BREAKING CHANGE: y") == "major"
    assert classify_message("fix: x", "BREAKING-CHANGE: y") == "major"
    assert classify_message("chore: x", "") == "none"
    assert classify_message("docs: x", "") == "none"
    assert classify_message("not conventional", "") == "none"
    assert bump_semver((0, 1, 0), "patch") == (0, 1, 1)
    assert bump_semver((0, 1, 9), "minor") == (0, 2, 0)
    assert bump_semver((0, 1, 0), "major") == (1, 0, 0)
    commits = [_commit("a1", "chore: x"), _commit("b2", "docs: y")]
    assert highest_bump(commits) == "none"
    assert highest_bump([_commit("a1", "feat: x"), _commit("b2", "fix: y")]) == "minor"

    table = default_type_table()
    assert table.order[:2] == ["breaking", "feat"], table.order
    assert table.is_hidden("docs") and not table.is_hidden("refactor")
    assert table.section("feat") == ("✨", "Features")

    sample = [
        _commit("aaaa11", "feat(feed): add webhook delivery"),
        _commit("bbbb22", "fix(client): handle empty response"),
        _commit("cccc33", "perf(DB): cut allocation"),
        _commit("dddd44", "feat!: drop legacy endpoint"),
        _commit("eeee55", "feat(admin): rotate tokens", "BREAKING CHANGE: old API is gone"),
        _commit("ffff66", "docs: rewrite the guide"),
        _commit("gggg77", "refactor: extract builder"),
        _commit("hhhh88", "deps: bump object store"),
        _commit("iiii99", "plain subject without a type"),
    ]
    notes = notes_markdown(
        "0.3.0", sample, baseline=False, table=table, pr_map={"bbbb22": "12"}
    )
    assert notes.startswith("## v0.3.0 — "), notes
    lines = notes.splitlines()

    def section_of(needle: str) -> int:
        for i, line in enumerate(lines):
            if needle in line:
                return i
        raise AssertionError(f"missing {needle!r} in:\n{notes}")

    assert section_of("### \U0001f4a5 Breaking Changes") < section_of("### ✨ Features")
    assert section_of("### ✨ Features") < section_of("### \U0001f41b Bug Fixes")
    assert section_of("### ⚡ Performance") < section_of("### ♻️ Refactoring")
    assert section_of("### ♻️ Refactoring") < section_of(f"### {OTHER_EMOJI} {OTHER_LABEL}")
    assert "- **admin** feat: rotate tokens (eeee55)" in lines
    assert "- feat!: " not in notes  # the bang marker is not repeated in the text
    assert "- **feed**: add webhook delivery (aaaa11)" in lines
    assert "- feat: drop legacy endpoint (dddd44)" in lines
    assert "- **db**: cut allocation (cccc33)" in lines, notes
    assert "- **client**: handle empty response (bbbb22) #12" in lines
    assert "- deps: bump object store" not in notes
    assert "- **deps**: bump object store" not in notes
    assert "- bump object store (hhhh88)" in lines
    assert "rewrite the guide" not in notes
    assert "plain subject" not in notes
    assert "- **feed**:" in notes and "**feed**:" in notes

    # Hiding a type removes its section and nothing else.
    hidden_fix = parse_type_lines(
        ["breaking=💥=Breaking Changes", "feat=✨=Features", "# fix=🐛=Bug Fixes", "perf=⚡=Performance"],
        "<fixture>",
    )
    masked = notes_markdown("0.3.0", sample, baseline=False, table=hidden_fix)
    assert "Bug Fixes" not in masked
    assert "Features" in masked and "Performance" in masked
    # fix entries fall through to Other once the type is unregistered-and-hidden:
    # hidden means "do not show", so the entry must not reappear anywhere.
    assert "handle empty response" not in masked

    # Reordering the configuration reorders the sections.
    swapped = parse_type_lines(["fix=🐛=Bug Fixes", "feat=✨=Features"], "<fixture>")
    notes2 = notes_markdown("0.3.0", sample, baseline=False, table=swapped).splitlines()
    pos_fix = next(i for i, l in enumerate(notes2) if "Bug Fixes" in l)
    pos_feat = next(i for i, l in enumerate(notes2) if "Features" in l)
    assert pos_fix < pos_feat, "\n".join(notes2)

    # Baseline replays nothing.
    base = notes_markdown("0.1.0", [], baseline=True)
    assert "Initial registry baseline" in base and "### " not in base

    # A range with no displayable commits says so.
    quiet = notes_markdown("0.1.1", [_commit("zz", "chore: tidy")], baseline=False, table=table)
    assert "No user-facing Conventional Commits in this range." in quiet
    assert "### " not in quiet

    # Bad configuration rows must not be ignored silently.
    for bad in ["Feat=✨=Features\n", "feat=\n", "=✨=No key\n", "chore=🔧\nchore=🛠\n"]:
        try:
            parse_type_lines(bad.splitlines(), "<fixture>")
        except SystemExit:
            continue
        raise AssertionError(f"expected a hard error for {bad!r}")

    # The shipped configuration must parse identically to the embedded default.
    shipped = Path(__file__).resolve().parent / "changelog-types.conf"
    if shipped.is_file():
        from_shipped = notes_markdown(
            "0.3.0", sample, baseline=False, table=type_table(), pr_map={"bbbb22": "12"}
        )
        assert from_shipped == notes, "scripts/changelog-types.conf diverges from the default"
        assert sorted(from_shipped.split()) == sorted(notes.split())

    # CHANGELOG.md is created when the branch has none.
    cl = Path("CHANGELOG.md")
    existed = cl.is_file()
    # Byte-for-byte restore: a text round-trip would rewrite CRLF files.
    original = cl.read_bytes() if existed else None
    try:
        if cl.is_file():
            cl.unlink()
        prepend_changelog("0.1.0", "## v0.1.0 — 2026-01-01\n\nInitial\n")
        text = cl.read_text(encoding="utf-8")
        assert text.startswith("# Changelog\n\n## v0.1.0"), text
        prepend_changelog("0.2.0", "## v0.2.0 — 2026-02-02\n\nSecond\n")
        text = cl.read_text(encoding="utf-8")
        assert text.startswith("# Changelog\n\n## v0.2.0"), text
        assert text.index("## v0.2.0") < text.index("## v0.1.0")
        cl.write_text("# Changelog\n\n## old\n", encoding="utf-8")
        prepend_changelog("0.3.0", "## v0.3.0 — X\n\nNewest\n")
        text = cl.read_text(encoding="utf-8")
        assert text.startswith("# Changelog\n\n## v0.3.0") and "## old" in text
    finally:
        if existed and original is not None:
            cl.write_bytes(original)
        elif cl.is_file():
            cl.unlink()

    # Manifest kinds round-trip.
    pom = "<artifactId>kirivers-client</artifactId>\n  <version>0.1.0</version>"
    tmp = Path("_sdk_rel_pom.xml")
    tmp.write_text(pom, encoding="utf-8")
    meta = {"kind": "pom", "path": str(tmp), "artifact": "kirivers-client"}
    assert read_manifest_version(meta) == "0.1.0"
    write_manifest_version(meta, "0.2.0")
    assert "0.2.0" in tmp.read_text(encoding="utf-8")
    tmp.unlink()

    cml = Path("_sdk_rel_CMakeLists.txt")
    cml.write_text(
        "cmake_minimum_required(VERSION 3.16)\n"
        "project(KiriVersCClient\n"
        "  LANGUAGES C\n"
        "  VERSION 0.4.1\n"
        ")\n"
        "add_library(kirivers_c client.c)\n",
        encoding="utf-8",
    )
    meta = {"kind": "cmake", "path": str(cml)}
    assert read_manifest_version(meta) == "0.4.1", read_manifest_version(meta)
    write_manifest_version(meta, "0.5.0")
    text = cml.read_text(encoding="utf-8")
    assert "VERSION 0.5.0" in text and "cmake_minimum_required(VERSION 3.16)" in text, text
    cml.unlink()

    print("self-check ok")


def main() -> int:
    p = argparse.ArgumentParser(description="KiriVers SDK release helper")
    sub = p.add_subparsers(dest="cmd", required=True)
    sp = sub.add_parser("plan")
    sp.add_argument("--write-notes", default="")
    sp.set_defaults(func=cmd_plan)
    sa = sub.add_parser("apply")
    sa.set_defaults(func=cmd_apply)
    sc = sub.add_parser("commitlint")
    sc.add_argument("--pr-title", default="")
    sc.add_argument("--base", default="")
    sc.add_argument("--head", default="HEAD")
    sc.set_defaults(func=cmd_commitlint)
    si = sub.add_parser("identity")
    si.set_defaults(func=cmd_identity)
    ss = sub.add_parser("self-check")
    ss.set_defaults(func=lambda _: (self_check() or 0))
    args = p.parse_args()
    return int(args.func(args) or 0)


if __name__ == "__main__":
    raise SystemExit(main())
