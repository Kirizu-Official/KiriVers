from __future__ import annotations

import hashlib
import json
from pathlib import Path

import pytest

from kirivers_client import Client, Config, HashMismatchError, PathFileStore, Updater
from kirivers_client.models import Integrity, IntegrityFile
from kirivers_client.updater import needed_paths_from_integrity
from tests.fakes import MockTransport, json_response


def _check_body(sha: str, url: str) -> dict:
    return {
        "has_update": True,
        "is_mandatory": False,
        "is_downgrade": False,
        "reason": "normal",
        "compare_engine": "semver",
        "version_integer": None,
        "version_semver": "1.1.0",
        "target_channel": "stable",
        "target_hw_rev": None,
        "package_type": "single_file",
        "root_hash": "",
        "package_url": url,
        "file_name": "app.bin",
        "size": 4,
        "sha256": sha,
        "delta_available": False,
    }


def test_updater_downloads_and_hashes(tmp_path: Path):
    from kirivers_client.transport import TransportResponse

    blob = b"test"
    digest = hashlib.sha256(blob).hexdigest()
    url = "/api/v1/projects/demo/packages/" + digest

    def handler(req):
        if req.url.endswith("/update/check"):
            return json_response(200, _check_body(digest, url))
        if req.url.endswith("/telemetry/report"):
            return json_response(202, {"status": "accepted"})
        if digest in req.url:
            return TransportResponse(status=200, headers={}, body=blob)
        raise AssertionError(req.url)

    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=MockTransport(handler=handler))
    updater = Updater(client, archive_unpacker=None, file_store=None, replacer=None)
    dest = tmp_path / "app.bin"
    result = updater.run(
        current_version="1.0.0",
        os="windows",
        arch="x86_64",
        dest_path=dest,
        apply=False,
        sleep=lambda _s: None,
    )
    assert result.outcome == "downloaded"
    assert dest.read_bytes() == blob
    assert result.sha256 == digest
    assert result.applied is False


def test_hash_mismatch(tmp_path: Path):
    from kirivers_client.transport import TransportResponse

    blob = b"nope"
    digest = "ab" * 32
    url = "/api/v1/projects/demo/packages/" + digest

    def handler(req):
        if req.url.endswith("/update/check"):
            return json_response(200, _check_body(digest, url))
        if req.url.endswith("/telemetry/report"):
            return json_response(202, {"status": "accepted"})
        return TransportResponse(status=200, headers={}, body=blob)

    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=MockTransport(handler=handler))
    updater = Updater(client, archive_unpacker=None, file_store=None, replacer=None)
    with pytest.raises(HashMismatchError):
        updater.run(
            current_version="1.0.0",
            os="windows",
            arch="x86_64",
            dest_path=tmp_path / "x.bin",
            apply=False,
        )


def test_keep_if_exists_omitted_from_needed_paths(tmp_path: Path):
    (tmp_path / "keep.txt").write_text("ok", encoding="utf-8")
    (tmp_path / "app.bin").write_bytes(b"old")
    store = PathFileStore(tmp_path)
    integrity = Integrity(
        files=[
            IntegrityFile(path="keep.txt", install_policy="KEEP_IF_EXISTS", sha256="00" * 32),
            IntegrityFile(
                path="app.bin",
                install_policy="OVERWRITE",
                sha256=hashlib.sha256(b"new").hexdigest(),
            ),
        ]
    )
    needed = needed_paths_from_integrity(integrity, store)
    assert needed == ["app.bin"]


def test_os_replace(tmp_path: Path):
    from kirivers_client.adapters import OsReplaceReplacer

    src = tmp_path / "a.bin"
    dst = tmp_path / "b.bin"
    src.write_bytes(b"x")
    OsReplaceReplacer().replace(str(src), str(dst))
    assert dst.read_bytes() == b"x"


def test_telemetry_network_error_does_not_fail_download(tmp_path: Path):
    from kirivers_client.transport import TransportResponse

    blob = b"test"
    digest = hashlib.sha256(blob).hexdigest()
    url = "/api/v1/projects/demo/packages/" + digest

    def handler(req):
        if req.url.endswith("/update/check"):
            return json_response(200, _check_body(digest, url))
        if req.url.endswith("/telemetry/report"):
            raise RuntimeError("transport down")
        if digest in req.url:
            return TransportResponse(status=200, headers={}, body=blob)
        raise AssertionError(req.url)

    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=MockTransport(handler=handler))
    updater = Updater(client, archive_unpacker=None, file_store=None, replacer=None)
    dest = tmp_path / "app.bin"
    result = updater.run(
        current_version="1.0.0",
        os="windows",
        arch="x86_64",
        dest_path=dest,
        apply=False,
    )
    assert result.outcome == "downloaded"
    assert dest.read_bytes() == blob


def test_unknown_delta_magic_falls_back_to_full_package(tmp_path: Path):
    from kirivers_client.transport import TransportResponse

    blob = b"full"
    digest = hashlib.sha256(blob).hexdigest()
    url = "/api/v1/projects/demo/packages/" + digest
    delta_url = "/api/v1/projects/demo/packages/" + ("11" * 32)
    current = tmp_path / "old.bin"
    current.write_bytes(b"old")

    def handler(req):
        if req.url.endswith("/update/check"):
            body = _check_body(digest, url)
            body["delta_available"] = True
            body["delta_algo"] = "bsdiff"
            return json_response(200, body)
        if req.url.endswith("/update/diff"):
            return json_response(
                200,
                {
                    "diff_mode": "binary_delta",
                    "root_hash": "",
                    "version_integer": None,
                    "version_semver": "1.1.0",
                    "channel": "stable",
                    "compare_engine": "semver",
                    "package_url": delta_url,
                    "delta_algo": "bsdiff",
                },
            )
        if req.url.endswith("/telemetry/report"):
            return json_response(202, {"status": "accepted"})
        if ("11" * 32) in req.url:
            return TransportResponse(status=200, headers={}, body=b"NOT-A-DELTA")
        if digest in req.url:
            return TransportResponse(status=200, headers={}, body=blob)
        raise AssertionError(req.url)

    class _Patcher:
        def supported_algos(self):
            return ["bsdiff"]

        def apply(self, old: bytes, delta: bytes) -> bytes:
            raise AssertionError("must not apply unknown magic")

    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=MockTransport(handler=handler))
    updater = Updater(client, patcher=_Patcher(), archive_unpacker=None, file_store=None, replacer=None)
    dest = tmp_path / "app.bin"
    result = updater.run(
        current_version="1.0.0",
        os="windows",
        arch="x86_64",
        dest_path=dest,
        current_file=current,
        apply=False,
    )
    assert result.diff_mode == "full_package"
    assert dest.read_bytes() == blob


def test_downgrade_does_not_call_diff(tmp_path: Path):
    from kirivers_client.transport import TransportResponse

    blob = b"full"
    digest = hashlib.sha256(blob).hexdigest()
    url = "/api/v1/projects/demo/packages/" + digest
    current = tmp_path / "old.bin"
    current.write_bytes(b"old")

    def handler(req):
        if req.url.endswith("/update/diff"):
            raise AssertionError("downgrade must not POST /update/diff")
        if req.url.endswith("/update/check"):
            body = _check_body(digest, url)
            body["is_downgrade"] = True
            body["delta_available"] = True
            return json_response(200, body)
        if req.url.endswith("/telemetry/report"):
            return json_response(202, {"status": "accepted"})
        if digest in req.url:
            return TransportResponse(status=200, headers={}, body=blob)
        raise AssertionError(req.url)

    class _Patcher:
        def supported_algos(self):
            return ["bsdiff"]

        def apply(self, old: bytes, delta: bytes) -> bytes:
            raise AssertionError("downgrade must not patch")

    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=MockTransport(handler=handler))
    updater = Updater(client, patcher=_Patcher(), archive_unpacker=None, file_store=None, replacer=None)
    dest = tmp_path / "app.bin"
    result = updater.run(
        current_version="1.1.0",
        os="windows",
        arch="x86_64",
        dest_path=dest,
        current_file=current,
        apply=False,
    )
    assert result.diff_mode == "full_package"


def test_updater_200_without_has_update_is_no_update(tmp_path: Path):
    def handler(req):
        if req.url.endswith("/update/check"):
            body = _check_body("ab" * 32, "/api/v1/projects/demo/packages/" + "ab" * 32)
            body["has_update"] = False
            return json_response(200, body)
        raise AssertionError(f"must not continue past check: {req.url}")

    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=MockTransport(handler=handler))
    updater = Updater(client, archive_unpacker=None, file_store=None, replacer=None)
    result = updater.run(
        current_version="1.0.0",
        os="windows",
        arch="x86_64",
        dest_path=tmp_path / "x.bin",
        apply=False,
    )
    assert result.outcome == "no_update"


def test_apply_replaces_from_staging_path(tmp_path: Path):
    from kirivers_client.transport import TransportResponse

    blob = b"test"
    digest = hashlib.sha256(blob).hexdigest()
    url = "/api/v1/projects/demo/packages/" + digest
    dest = tmp_path / "app.bin"

    class RecordingReplacer:
        def __init__(self) -> None:
            self.source = ""
            self.destination = ""

        def replace(self, source: str, destination: str) -> None:
            self.source = source
            self.destination = destination
            Path(destination).write_bytes(Path(source).read_bytes())
            Path(source).unlink()

    def handler(req):
        if req.url.endswith("/update/check"):
            return json_response(200, _check_body(digest, url))
        if req.url.endswith("/telemetry/report"):
            return json_response(202, {"status": "accepted"})
        if digest in req.url:
            return TransportResponse(status=200, headers={}, body=blob)
        raise AssertionError(req.url)

    rec = RecordingReplacer()
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=MockTransport(handler=handler))
    updater = Updater(client, archive_unpacker=None, file_store=None, replacer=rec)
    result = updater.run(
        current_version="1.0.0",
        os="windows",
        arch="x86_64",
        dest_path=dest,
        apply=True,
    )
    assert result.applied is True
    assert rec.destination == str(dest)
    assert rec.source != str(dest)
    assert rec.source.endswith(".kv-partial")
    assert dest.read_bytes() == blob
    assert not Path(rec.source).exists()


def test_via_pack_uses_injected_file_store(tmp_path: Path):
    from kirivers_client.transport import TransportResponse

    blob = b"pack-bytes"
    digest = hashlib.sha256(blob).hexdigest()
    url = "/api/v1/projects/demo/packages/" + digest
    seen: list[str] = []

    class RecordingStore:
        def can_write_individual_files(self) -> bool:
            return True

        def exists(self, relative_path: str) -> bool:
            seen.append(relative_path)
            return relative_path == "keep.txt"

        def read_bytes(self, relative_path: str) -> bytes:
            return b"ok"

        def write_bytes(self, relative_path: str, data: bytes) -> None:
            return None

        def sha256_hex(self, relative_path: str) -> str:
            return "00" * 32

    def handler(req):
        if req.url.endswith("/update/check"):
            body = _check_body(digest, url)
            body["package_type"] = "multi_file"
            return json_response(200, body)
        if "/integrity" in req.url:
            return json_response(
                200,
                {
                    "version_semver": "1.1.0",
                    "package_type": "multi_file",
                    "files": [
                        {"path": "keep.txt", "install_policy": "KEEP_IF_EXISTS", "sha256": "00" * 32},
                        {"path": "app.bin", "install_policy": "OVERWRITE", "sha256": "11" * 32},
                    ],
                },
            )
        if req.url.endswith("/update/pack"):
            payload = json.loads(req.body.decode("utf-8"))
            assert payload["needed_paths"] == ["app.bin"]
            return json_response(200, {"status": "ready", "package_url": url, "sha256": digest, "files": []})
        if req.url.endswith("/telemetry/report"):
            return json_response(202, {"status": "accepted"})
        if digest in req.url:
            return TransportResponse(status=200, headers={}, body=blob)
        raise AssertionError(req.url)

    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=MockTransport(handler=handler))
    updater = Updater(client, file_store=RecordingStore(), archive_unpacker=None, replacer=None)
    dest = tmp_path / "pkg.bin"
    result = updater.run(
        current_version="1.0.0",
        os="windows",
        arch="x86_64",
        dest_path=dest,
        install_dir=tmp_path,
        apply=False,
        sleep=lambda _s: None,
    )
    assert result.outcome == "downloaded"
    assert "keep.txt" in seen
    assert "app.bin" in seen
    assert dest.read_bytes() == blob
