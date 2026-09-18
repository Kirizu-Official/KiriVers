from __future__ import annotations

import json

import pytest

from kirivers_client import Client, Config
from kirivers_client.errors import APIError
from tests.fakes import MockTransport, json_response


def test_pack_poll_repeats_identical_json_body():
    pending = json_response(202, {"status": "pending"})
    ready = json_response(200, {"status": "ready", "package_url": "/p", "sha256": "aa" * 32})
    n = {"i": 0}

    def handler(req):
        n["i"] += 1
        return pending if n["i"] == 1 else ready

    transport = MockTransport(handler=handler)
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    result = client.pack_until_ready(
        source_version="1.0.0",
        target_version="1.1.0",
        os="windows",
        arch="x86_64",
        needed_paths=["bin/app"],
        sleep=lambda _s: None,
        clock=lambda: 0.0 if n["i"] < 2 else 0.0,
        deadline_s=30,
    )
    assert result.status == "ready"
    assert len(transport.calls) == 2
    assert transport.calls[0].body == transport.calls[1].body
    assert transport.calls[0].method == "POST"
    assert transport.calls[0].url.endswith("/update/pack")
    payload = json.loads(transport.calls[0].body.decode("utf-8"))
    assert payload["needed_paths"] == ["bin/app"]
    assert "job_id" not in payload


def test_pack_sends_empty_needed_paths_array():
    transport = MockTransport(responses=json_response(200, {"status": "ready"}))
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    client.pack(
        source_version="1.0.0",
        target_version="1.1.0",
        os="windows",
        arch="x86_64",
        needed_paths=[],
    )
    payload = json.loads(transport.calls[0].body.decode("utf-8"))
    assert payload["needed_paths"] == []


def test_pack_omits_needed_paths_when_unspecified():
    transport = MockTransport(responses=json_response(200, {"status": "ready"}))
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    client.pack(
        source_version="1.0.0",
        target_version="1.1.0",
        os="windows",
        arch="x86_64",
    )
    payload = json.loads(transport.calls[0].body.decode("utf-8"))
    assert "needed_paths" not in payload


def test_pack_timeout():
    transport = MockTransport(responses=json_response(202, {"status": "pending"}))
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)

    class Clock:
        def __init__(self) -> None:
            self.t = 0.0

        def __call__(self) -> float:
            return self.t

    clock = Clock()

    def sleep(seconds: float) -> None:
        clock.t += seconds

    with pytest.raises(APIError) as caught:
        client.pack_until_ready(
            source_version="1.0.0",
            target_version="1.1.0",
            os="windows",
            arch="x86_64",
            deadline_s=3,
            initial_wait_s=1,
            max_wait_s=1,
            sleep=sleep,
            clock=clock,
        )
    assert caught.value.code == "PACK_TIMEOUT"


def test_telemetry_202():
    transport = MockTransport(responses=json_response(202, {"status": "accepted"}))
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    out = client.report_telemetry(
        os="windows",
        arch="x86_64",
        channel="stable",
        from_version="1.0.0",
        to_version="1.1.0",
        status="installed",
        device_id="secret-device",
    )
    assert out.get("status") == "accepted"
    body = transport.calls[0].body.decode("utf-8")
    payload = json.loads(body)
    assert payload["device_id"] == "secret-device"
    assert payload["status"] == "installed"
