#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""SDK release planner: Conventional Commits bump, prefixed tags, optional changelog.

Used by GitHub Actions and GitLab CI on each language package root.
Stdlib only. Commands: plan | apply | commitlint | identity
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

SEMVER = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")
SUBJECT = re.compile(
    r"^(?P<type>feat|fix|perf|revert|chore|docs|ci|test|style|refactor|build)"
    r"(?:\([^)]+\))?(?P<bang>!)?:\s+.+"
)
RELEASE_TYPES = {"feat": "minor", "fix": "patch", "perf": "patch", "revert": "patch"}
RANK = {"none": 0, "patch": 1, "minor": 2, "major": 3}

# SDK_LANG → tag prefix, manifest, whether publish is only allowed from a D12 repo.
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
    gh = os.environ.get("GITHUB_REPOSITORY", "").strip()
    gl = os.environ.get("CI_PROJECT_PATH", "").strip()
    want_s = str(want)
    return gh == want_s or gl == want_s or gl.lower() == want_s.lower()


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


def classify_message(subject: str, body: str) -> str:
    """Return none|patch|minor|major for one commit."""
    subj = subject.strip()
    m = SUBJECT.match(subj)
    if not m:
        return "none"
    breaking = bool(m.group("bang")) or "BREAKING CHANGE:" in body or "BREAKING-CHANGE:" in body
    if breaking:
        return "major"
    return RELEASE_TYPES.get(m.group("type"), "none")


def collect_commits(since_tag: str | None) -> list[tuple[str, str]]:
    spec = f"{since_tag}..HEAD" if since_tag else ""
    args = ["log", "--format=%s%x1f%b%x1e", "--no-merges"]
    if spec:
        args.append(spec)
    raw = git(*args)
    out: list[tuple[str, str]] = []
    for chunk in raw.split("\x1e"):
        chunk = chunk.strip("\n")
        if not chunk.strip():
            continue
        if "\x1f" in chunk:
            subj, body = chunk.split("\x1f", 1)
        else:
            subj, body = chunk, ""
        out.append((subj.strip(), body.strip()))
    return out


def highest_bump(commits: list[tuple[str, str]]) -> str:
    best = "none"
    for subj, body in commits:
        kind = classify_message(subj, body)
        if RANK[kind] > RANK[best]:
            best = kind
    return best


def notes_markdown(version: str, commits: list[tuple[str, str]], baseline: bool) -> str:
    if baseline:
        return f"## {version}\n\nInitial registry baseline (manifest version, no commit replay).\n"
    groups: dict[str, list[str]] = {
        "Breaking Changes": [],
        "Features": [],
        "Bug Fixes": [],
        "Performance": [],
        "Reverts": [],
    }
    for subj, body in commits:
        kind = classify_message(subj, body)
        if kind == "none":
            continue
        m = SUBJECT.match(subj.strip())
        title = subj.split(":", 1)[-1].strip() if ":" in subj else subj
        if m and m.group("type") == "feat" and kind != "major":
            groups["Features"].append(title)
        elif m and m.group("type") == "fix":
            groups["Bug Fixes"].append(title)
        elif m and m.group("type") == "perf":
            groups["Performance"].append(title)
        elif m and m.group("type") == "revert":
            groups["Reverts"].append(title)
        if kind == "major":
            groups["Breaking Changes"].append(title)
    lines = [f"## {version}", ""]
    for heading, items in groups.items():
        if not items:
            continue
        lines.append(f"### {heading}")
        for it in items:
            lines.append(f"- {it}")
        lines.append("")
    if len(lines) == 2:
        lines.append("No user-facing Conventional Commits in this range.")
        lines.append("")
    return "\n".join(lines)


def prepend_changelog(version: str, notes: str) -> None:
    path = Path("CHANGELOG.md")
    if not path.is_file():
        return
    existing = path.read_text(encoding="utf-8")
    block = notes.rstrip() + "\n\n"
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
    result["notes"] = notes_markdown(new_ver, commits, baseline=False)
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
        "(feat|fix|perf|revert|chore|docs|ci|test|style|refactor|build)"
    )


def cmd_commitlint(args: argparse.Namespace) -> int:
    errors: list[str] = []
    if args.pr_title:
        err = lint_one("PR/MR title", args.pr_title)
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
                "gitlab": os.environ.get("CI_PROJECT_PATH", ""),
                "want": meta.get("publish_repo"),
            },
            indent=2,
        )
    )
    return 0 if ok else 2


def self_check() -> None:
    assert classify_message("feat: x", "") == "minor"
    assert classify_message("fix: x", "") == "patch"
    assert classify_message("feat!: x", "") == "major"
    assert classify_message("feat: x", "BREAKING CHANGE: y") == "major"
    assert classify_message("chore: x", "") == "none"
    assert classify_message("docs: x", "") == "none"
    assert classify_message("not conventional", "") == "none"
    assert bump_semver((0, 1, 0), "patch") == (0, 1, 1)
    assert bump_semver((0, 1, 9), "minor") == (0, 2, 0)
    assert bump_semver((0, 1, 0), "major") == (1, 0, 0)
    assert highest_bump([("chore: x", ""), ("docs: y", "")]) == "none"
    assert highest_bump([("feat: x", ""), ("fix: y", "")]) == "minor"
    cl = Path("CHANGELOG.md")
    existed = cl.is_file()
    original = cl.read_text(encoding="utf-8") if existed else None
    try:
        cl.write_text("# Changelog\n\n## old\n", encoding="utf-8")
        prepend_changelog("0.1.0", "## 0.1.0\n\nInitial\n")
        text = cl.read_text(encoding="utf-8")
        assert text.startswith("# Changelog")
        assert text.index("## 0.1.0") < text.index("## old")
    finally:
        if existed and original is not None:
            cl.write_text(original, encoding="utf-8")
        elif cl.is_file():
            cl.unlink()
    pom = "<artifactId>kirivers-client</artifactId>\n  <version>0.1.0</version>"
    tmp = Path("_sdk_rel_pom.xml")
    tmp.write_text(pom, encoding="utf-8")
    meta = {"kind": "pom", "path": str(tmp), "artifact": "kirivers-client"}
    assert read_manifest_version(meta) == "0.1.0"
    write_manifest_version(meta, "0.2.0")
    assert "0.2.0" in tmp.read_text(encoding="utf-8")
    tmp.unlink()
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
