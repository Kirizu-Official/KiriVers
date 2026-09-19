"""OpenAPI contract tests: mock Transport only, no Docker."""

from __future__ import annotations

import json
from pathlib import Path
from urllib.parse import parse_qs, urlparse

import pytest

from kirivers_client import Client, Config
from kirivers_client.errors import APIError
from tests.fakes import MockTransport, empty_response, json_response

ROOT = Path(__file__).resolve().parents[1]
OPENAPI = json.loads((ROOT / "openapi.client.json").read_text(encoding="utf-8"))

SKIP_PATH_PREFIXES = ("/api/v1/projects/{project_ref}/store/",)
SKIP_PATHS = {"/api/v1/openapi.json"}
SKIP_METHODS = {"options"}

LEFTOVER_SUBSTRINGS = (
    "/update/pack/status",
    "/clients/login",
    "/manifest",
    "/artifacts/",
    "/store/",
    "/api/v1/ready",
)

NATIVE_OPS: list[tuple[str, str, str, dict]] = [
    ("GET", "/api/v1/health", "health", {}),
    ("GET", "/api/v1/projects/{project_ref}", "project", {}),
    ("POST", "/api/v1/projects/{project_ref}/clients/report", "device_report", {"device_id": "dev-1"}),
    (
        "POST",
        "/api/v1/projects/{project_ref}/update/check",
        "check",
        {"current_version": "1.0.0", "os": "windows", "arch": "x86_64"},
    ),
    (
        "GET",
        "/api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}",
        "changelog",
        {"channel": "stable", "os": "windows", "arch": "x86_64"},
    ),
    (
        "GET",
        "/api/v1/projects/{project_ref}/versions/{version}/integrity",
        "integrity",
        {"version": "1.1.0", "os": "windows", "arch": "x86_64"},
    ),
    (
        "POST",
        "/api/v1/projects/{project_ref}/update/diff",
        "diff",
        {
            "source_version": "1.0.0",
            "target_version": "1.1.0",
            "os": "windows",
            "arch": "x86_64",
            "local_sha256": "aa" * 32,
        },
    ),
    (
        "POST",
        "/api/v1/projects/{project_ref}/update/pack",
        "pack",
        {
            "source_version": "1.0.0",
            "target_version": "1.1.0",
            "os": "windows",
            "arch": "x86_64",
            "needed_paths": ["bin/app"],
        },
    ),
    ("GET", "/api/v1/projects/{project_ref}/packages/{ref}", "download", {"ref": "ab" * 32}),
    ("HEAD", "/api/v1/projects/{project_ref}/packages/{ref}", "head_package", {"ref": "ab" * 32}),
    ("GET", "/api/v1/projects/{project_ref}/channels", "channels", {}),
    ("GET", "/api/v1/projects/{project_ref}/matrix", "matrix", {}),
    ("GET", "/api/v1/projects/{project_ref}/languages", "languages", {}),
    ("GET", "/api/v1/projects/{project_ref}/announcements", "announcements", {}),
    (
        "POST",
        "/api/v1/projects/{project_ref}/telemetry/report",
        "report_telemetry",
        {
            "os": "windows",
            "arch": "x86_64",
            "channel": "stable",
            "from_version": "1.0.0",
            "to_version": "1.1.0",
            "status": "installed",
        },
    ),
    ("GET", "/api/v1/projects/{project_ref}/media/{id}", "media", {"media_id": "11111111-1111-1111-1111-111111111111"}),
    ("HEAD", "/api/v1/projects/{project_ref}/media/{id}", "head_media", {"media_id": "11111111-1111-1111-1111-111111111111"}),
]


def _openapi_native_ops() -> set[tuple[str, str]]:
    out: set[tuple[str, str]] = set()
    for path, item in OPENAPI["paths"].items():
        if path in SKIP_PATHS or any(path.startswith(p) for p in SKIP_PATH_PREFIXES):
            continue
        for method, spec in item.items():
            if method.startswith("x-") or method in SKIP_METHODS:
                continue
            if not isinstance(spec, dict):
                continue
            out.add((method.upper(), path))
    return out


def _required_body_fields(method: str, path: str) -> list[str]:
    spec = OPENAPI["paths"][path][method.lower()]
    body = spec.get("requestBody") or {}
    content = ((body.get("content") or {}).get("application/json") or {}).get("schema") or {}
    if "$ref" in content:
        name = content["$ref"].rsplit("/", 1)[-1]
        content = OPENAPI["components"]["schemas"][name]
    return list(content.get("required") or [])


def _required_query(method: str, path: str) -> list[str]:
    spec = OPENAPI["paths"][path][method.lower()]
    names = []
    for param in spec.get("parameters") or []:
        if "$ref" in param:
            continue
        if param.get("in") == "query" and param.get("required"):
            names.append(param["name"])
    return names


def _ok_payload(method: str, path: str) -> dict:
    if path.endswith("/health"):
        return {"status": "ok", "ready": True}
    if path.endswith("/clients/report"):
        return {"ip": "127.0.0.1", "country_code": "", "region_code": "", "geo_i18n": {}}
    if path.endswith("/update/check"):
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
            "package_url": "/api/v1/projects/demo/packages/" + "ab" * 32,
            "file_name": "app.bin",
            "size": 1,
            "sha256": "ab" * 32,
            "delta_available": False,
        }
    if "/changelog/" in path:
        return {"changelog": "# hi", "changelog_versions": []}
    if path.endswith("/integrity"):
        return {
            "version_integer": None,
            "version_semver": "1.1.0",
            "channel": "stable",
            "package_type": "single_file",
            "root_hash": "",
            "full_package_url": "/p",
            "file_name": "app.bin",
            "size": 1,
            "sha256": "ab" * 32,
            "files": [{"path": "app.bin", "size": 1, "install_policy": "OVERWRITE", "integrity_check": True}],
        }
    if path.endswith("/update/diff"):
        return {
            "diff_mode": "full_package",
            "root_hash": "",
            "version_integer": None,
            "version_semver": "1.1.0",
            "channel": "stable",
            "compare_engine": "semver",
        }
    if path.endswith("/update/pack"):
        return {"status": "ready", "package_url": "/p", "sha256": "ab" * 32}
    if path.endswith("/channels"):
        return {"channels": [{"slug": "stable", "name": "Stable", "stability_rank": 30}]}
    if path.endswith("/matrix"):
        return {"matrix": [{"os": "windows", "arch": "x86_64", "package_type": "single_file"}]}
    if path.endswith("/languages"):
        return {"languages": [{"code": "en", "display_name": "English", "is_default": True, "sort_order": 0}]}
    if path.endswith("/announcements"):
        return {"announcements": []}
    if path.endswith("/telemetry/report"):
        return {"status": "accepted"}
    if path.endswith("}") and "projects/{project_ref}" == path:
        return {"slug": "demo", "uuid": "00000000-0000-0000-0000-000000000000"}
    return {}


def test_openapi_native_surface_matches_client():
    documented = _openapi_native_ops()
    implemented = {(m, p) for m, p, *_ in NATIVE_OPS}
    missing = documented - implemented
    extra = implemented - documented
    assert not missing, f"Client missing OpenAPI ops: {sorted(missing)}"
    assert not extra, f"Client extra ops not in OpenAPI native set: {sorted(extra)}"


@pytest.mark.parametrize("method,path,name,kwargs", NATIVE_OPS)
def test_client_emits_path_method_and_required_fields(method, path, name, kwargs):
    def handler(req):
        if method in ("GET", "HEAD") and ("/packages/" in path or "/media/" in path):
            return empty_response(200, {"Content-Type": "application/octet-stream"})
        if name == "report_telemetry":
            return json_response(202, {"status": "accepted"})
        if name == "pack":
            return json_response(200, _ok_payload(method, path))
        return json_response(200, _ok_payload(method, path))

    transport = MockTransport(handler=handler)
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    getattr(client, name)(**kwargs)
    assert transport.calls, name
    call = transport.calls[0]
    assert call.method == method
    parsed = urlparse(call.url)
    expected_path = path.replace("{project_ref}", "demo")
    for key, value in kwargs.items():
        token = "{" + ("id" if key == "media_id" else "ref" if key == "ref" else "version" if key == "version" else key) + "}"
        if token in expected_path:
            expected_path = expected_path.replace(token, str(value))
    # changelog uses channel/os/arch path params
    if "{channel}" in expected_path:
        expected_path = expected_path.replace("{channel}", kwargs["channel"]).replace("{os}", kwargs["os"]).replace("{arch}", kwargs["arch"])
    assert parsed.path == expected_path
    for leftover in LEFTOVER_SUBSTRINGS:
        assert leftover not in call.url
    if method == "GET" and path.endswith("/update/check"):
        raise AssertionError("GET /update/check must not be implemented")
    required_body = _required_body_fields(method, path)
    if required_body:
        assert call.body
        payload = json.loads(call.body.decode("utf-8"))
        for field in required_body:
            assert field in payload, field
            assert payload[field] not in (None, "", [])
    for qname in _required_query(method, path):
        qs = parse_qs(parsed.query)
        assert qname in qs and qs[qname][0], qname


def test_check_required_json_names_and_default_capabilities():
    transport = MockTransport(responses=json_response(204, {}))
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    outcome = client.check(current_version="1.0.0", os="windows", arch="x86_64")
    assert outcome.no_update
    payload = json.loads(transport.calls[0].body.decode("utf-8"))
    assert payload["current_version"] == "1.0.0"
    assert payload["os"] == "windows"
    assert payload["arch"] == "x86_64"
    assert payload["capabilities"] == ["full_package"]
    assert "accepted_delta_algos" not in payload
    assert "local_sha256" not in payload
    assert "dirty_paths" not in payload
    assert "changelog" not in payload


def test_check_204_is_not_error_and_304_is_etag():
    t204 = MockTransport(responses=empty_response(204, {"ETag": '"abc"'}))
    c204 = Client(Config(base_url="http://example.test", project_ref="demo"), transport=t204)
    r204 = c204.check(current_version="1.0.0", os="windows", arch="x86_64")
    assert r204.status == 204 and r204.no_update and r204.etag == '"abc"'

    t304 = MockTransport(responses=empty_response(304, {"ETag": '"abc"'}))
    c304 = Client(Config(base_url="http://example.test", project_ref="demo"), transport=t304)
    r304 = c304.check(current_version="1.0.0", os="windows", arch="x86_64", etag='"abc"')
    assert r304.not_modified
    assert t304.calls[0].headers.get("If-None-Match") == '"abc"'


def test_error_envelope_preserves_unknown_code():
    transport = MockTransport(
        responses=json_response(
            404,
            {"error": {"code": "SOME_NEW_CODE", "message": "nope", "details": {"k": 1}}},
        )
    )
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    with pytest.raises(APIError) as caught:
        client.project()
    err = caught.value
    assert err.status == 404
    assert err.code == "SOME_NEW_CODE"
    assert err.message == "nope"
    assert err.details == {"k": 1}


def test_device_report_response_is_geo_only():
    transport = MockTransport(
        responses=json_response(
            200,
            {
                "ip": "127.0.0.1",
                "country_code": "US",
                "region_code": "CA",
                "geo_i18n": {"en": "California"},
                "id": "roster-uuid",
                "device_id": "raw-secret",
                "device_hash": "abc",
            },
        )
    )
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    report = client.device_report("dev-1")
    assert report.ip == "127.0.0.1"
    assert report.country_code == "US"
    assert not hasattr(report, "device_id")
    assert set(report.__dataclass_fields__) == {"ip", "country_code", "region_code", "geo_i18n"}


def test_rate_limited_retry_after():
    transport = MockTransport(
        responses=json_response(
            429,
            {"error": {"code": "RATE_LIMITED", "message": "slow down"}},
            extra_headers={"Retry-After": "7"},
        )
    )
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    with pytest.raises(APIError) as caught:
        client.channels()
    assert caught.value.retry_after == 7


def test_no_leftover_methods_on_client():
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=MockTransport(responses=empty_response(200)))
    for name in ("login", "get_check", "pack_status", "manifest", "store"):
        assert not hasattr(client, name)


def test_openapi_omits_leftover_and_check_is_post_only():
    paths = OPENAPI["paths"]
    assert "get" not in paths["/api/v1/projects/{project_ref}/update/check"]
    assert "/api/v1/projects/{project_ref}/clients/login" not in paths
    assert "/api/v1/projects/{project_ref}/update/pack/status" not in paths
    assert "/api/v1/ready" not in paths
    err = OPENAPI["components"]["schemas"]["Error"]["properties"]["error"]["properties"]
    assert {"code", "message", "details"} <= set(err)


def test_download_keeps_exp_sig_and_range():
    transport = MockTransport(responses=empty_response(206, {"Content-Type": "application/octet-stream"}))
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    client.download("aa" * 32, byte_range="bytes=0-10", exp="99", sig="deadbeef")
    url = transport.calls[0].url
    qs = parse_qs(urlparse(url).query)
    assert qs["exp"] == ["99"] and qs["sig"] == ["deadbeef"]
    assert transport.calls[0].headers.get("Range") == "bytes=0-10"
    assert transport.calls[0].method == "GET"


def test_download_url_preserves_signed_query():
    transport = MockTransport(responses=empty_response(200))
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    client.download_url("/api/v1/projects/demo/packages/" + "aa" * 32 + "?exp=1&sig=zz")
    parsed = urlparse(transport.calls[0].url)
    assert parsed.query == "exp=1&sig=zz"
    assert parsed.netloc == "example.test"


def test_download_url_omits_tokens_on_foreign_origin():
    transport = MockTransport(responses=empty_response(200))
    client = Client(
        Config(
            base_url="http://example.test",
            project_ref="demo",
            project_token="secret",
            channel_token="chan",
        ),
        transport=transport,
    )
    client.download_url("https://cdn.example/obj.bin")
    headers = {k.lower(): v for k, v in transport.calls[0].headers.items()}
    assert "authorization" not in headers
    assert "x-project-token" not in headers
    assert "x-channel-token" not in headers
    assert headers.get("user-agent")


def test_same_origin_download_keeps_project_token():
    transport = MockTransport(responses=empty_response(200))
    client = Client(
        Config(base_url="http://example.test", project_ref="demo", project_token="secret"),
        transport=transport,
    )
    client.download("aa" * 32)
    headers = {k.lower(): v for k, v in transport.calls[0].headers.items()}
    assert headers.get("authorization") == "Bearer secret"
    assert headers.get("x-project-token") == "secret"


def test_integrity_requires_os_arch_query():
    transport = MockTransport(
        responses=json_response(
            200,
            {
                "version_integer": None,
                "version_semver": "1.0.0",
                "channel": "stable",
                "package_type": "multi_file",
                "root_hash": "r",
                "full_package_url": "/p",
                "file_name": "a.zip",
                "size": 1,
                "sha256": "ab" * 32,
                "files": [],
            },
        )
    )
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    client.integrity("1.0.0", os="linux", arch="arm64")
    qs = parse_qs(urlparse(transport.calls[0].url).query)
    assert qs["os"] == ["linux"] and qs["arch"] == ["arm64"]


def test_diff_sends_local_sha256_not_on_check_path():
    transport = MockTransport(
        responses=json_response(
            200,
            {
                "diff_mode": "binary_delta",
                "root_hash": "",
                "version_integer": None,
                "version_semver": "1.1.0",
                "channel": "stable",
                "compare_engine": "semver",
                "delta_algo": "bsdiff",
            },
        )
    )
    client = Client(Config(base_url="http://example.test", project_ref="demo"), transport=transport)
    client.diff(
        source_version="1.0.0",
        target_version="1.1.0",
        os="windows",
        arch="x86_64",
        local_sha256="cd" * 32,
    )
    payload = json.loads(transport.calls[0].body.decode("utf-8"))
    assert payload["local_sha256"] == "cd" * 32
    assert "/update/diff" in transport.calls[0].url
    assert "/update/check" not in transport.calls[0].url
