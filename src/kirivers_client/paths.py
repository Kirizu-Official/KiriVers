"""Relative-path rules aligned with server `pkg/pathutil` (slash + Unicode NFC)."""

from __future__ import annotations

import re
import unicodedata

from kirivers_client.errors import PathError

_DRIVE = re.compile(r"^[a-zA-Z]:")
_SLASHES = re.compile(r"/+")


def normalize_path(raw: str) -> str:
    """Normalize a fileset path: `\\` → `/`, Unicode NFC, no `..`, no drive letters.

    Raises:
        PathError: empty, absolute, traversal, control characters, or drive letter.
    """
    trimmed = raw.strip()
    if not trimmed:
        raise PathError("path is empty")
    for ch in trimmed:
        if ord(ch) < 0x20:
            raise PathError(f"contains control character 0x{ord(ch):02x}")
    if trimmed.startswith("/") or trimmed.startswith("\\"):
        raise PathError("path cannot start with leading slash")
    if _DRIVE.match(trimmed):
        raise PathError("path cannot contain drive letter")
    slash_unified = trimmed.replace("\\", "/")
    for seg in slash_unified.split("/"):
        if _DRIVE.match(seg):
            raise PathError("segment cannot contain drive letter")
        if seg in (".", ".."):
            raise PathError(f"path traversal segment {seg!r} is forbidden")
    nfc = unicodedata.normalize("NFC", slash_unified)
    cleaned = _SLASHES.sub("/", nfc).strip("/")
    if not cleaned:
        raise PathError("path normalized to empty")
    for seg in cleaned.split("/"):
        if seg in (".", "..", ""):
            raise PathError(f"invalid segment in normalized path {cleaned!r}")
    return cleaned


def unique_needed_paths(paths: list[str]) -> list[str]:
    """NFC-normalize, drop invalid/duplicate paths. Order is not fileset identity."""
    seen: set[str] = set()
    out: list[str] = []
    for raw in paths:
        try:
            norm = normalize_path(raw)
        except PathError:
            continue
        if norm not in seen:
            seen.add(norm)
            out.append(norm)
    return out
