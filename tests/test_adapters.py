from __future__ import annotations

import json

from kirivers_client import Client, Config, Updater, derive_capabilities
from kirivers_client.adapters import PathFileStore, ZipArchiveUnpacker
from tests.fakes import MockTransport, json_response


class _Patcher:
    def supported_algos(self):
        return ["bsdiff", "xdelta3"]

    def apply(self, old: bytes, delta: bytes) -> bytes:
        return old


def test_default_client_check_capabilities_full_package_only():
    transport = MockTransport(responses=json_response(204, {}))
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    client.check(current_version="1.0.0", os="windows", arch="x86_64")
    payload = json.loads(transport.calls[0].body.decode("utf-8"))
    assert payload["capabilities"] == ["full_package"]
    assert "accepted_delta_algos" not in payload


def test_updater_defaults_advertise_zip_and_file_list_not_delta(tmp_path):
    caps, algos = derive_capabilities(
        patcher=None,
        unpacker=ZipArchiveUnpacker(),
        file_store=PathFileStore(tmp_path),
    )
    assert caps == ["full_package", "patch_package", "file_list"]
    assert algos == []


def test_file_store_without_root_does_not_advertise_file_list():
    caps, _ = derive_capabilities(
        patcher=None,
        unpacker=ZipArchiveUnpacker(),
        file_store=PathFileStore(),
    )
    assert caps == ["full_package", "patch_package"]


def test_updater_with_patcher_sends_binary_delta_algos():
    transport = MockTransport(responses=json_response(204, {}))
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    updater = Updater(client, patcher=_Patcher())
    updater.run(current_version="1.0.0", os="windows", arch="x86_64")
    payload = json.loads(transport.calls[0].body.decode("utf-8"))
    assert "binary_delta" in payload["capabilities"]
    assert payload["accepted_delta_algos"] == ["bsdiff", "xdelta3"]
    assert "full_package" in payload["capabilities"]


def test_updater_install_dir_advertises_file_list(tmp_path):
    transport = MockTransport(responses=json_response(204, {}))
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    updater = Updater(client)
    updater.run(current_version="1.0.0", os="windows", arch="x86_64", install_dir=tmp_path)
    payload = json.loads(transport.calls[0].body.decode("utf-8"))
    assert "file_list" in payload["capabilities"]


def test_updater_without_unpacker_omits_patch_package():
    caps, _ = derive_capabilities(patcher=None, unpacker=None, file_store=None)
    assert caps == ["full_package"]


class _EmptyPatcher:
    def supported_algos(self):
        return ["", "  "]

    def apply(self, old: bytes, delta: bytes) -> bytes:
        return old


def test_empty_patcher_algos_do_not_advertise_binary_delta():
    caps, algos = derive_capabilities(
        patcher=_EmptyPatcher(),
        unpacker=None,
        file_store=None,
    )
    assert caps == ["full_package"]
    assert algos == []
