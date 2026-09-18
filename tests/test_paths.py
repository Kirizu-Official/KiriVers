from __future__ import annotations

import unicodedata

import pytest

from kirivers_client.errors import PathError
from kirivers_client.paths import normalize_path, unique_needed_paths


def test_backslash_and_nfc_collapse():
    nfd = unicodedata.normalize("NFD", "café")
    nfc = unicodedata.normalize("NFC", "café")
    assert nfd != nfc
    assert normalize_path(f"dir\\{nfd}\\file.bin") == f"dir/{nfc}/file.bin"


def test_reject_dotdot_and_absolute_and_drive():
    with pytest.raises(PathError):
        normalize_path("../etc/passwd")
    with pytest.raises(PathError):
        normalize_path("/abs")
    with pytest.raises(PathError):
        normalize_path("C:windows")
    with pytest.raises(PathError):
        normalize_path("foo/../bar")


def test_unique_needed_paths_dedupes_slash_variants():
    got = unique_needed_paths(["a\\b", "a/b", "a/b"])
    assert got == ["a/b"]
