from __future__ import annotations

from pathlib import Path

import pytest

from kirivers_client import Client, Config

ROOT = Path(__file__).resolve().parents[1]
OPENAPI_PATH = ROOT / "openapi.client.json"


@pytest.fixture
def client_factory():
    def _make(transport) -> Client:
        return Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)

    return _make
