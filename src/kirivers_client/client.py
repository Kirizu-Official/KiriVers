"""Handwritten native JSON client for the KiriVers client plane."""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any, Mapping
from urllib.parse import quote, urlencode, urljoin, urlparse

from kirivers_client.errors import APIError, ConfigError
from kirivers_client.models import (
    Announcement,
    BinaryResult,
    Channel,
    Changelog,
    CheckOutcome,
    DeviceReport,
    Diff,
    EnvelopeResult,
    Health,
    Integrity,
    Language,
    MatrixRow,
    Pack,
    ProjectPublic,
    UpdateCheck,
    omit_empty,
)
from kirivers_client.transport import RequestsTransport, Transport, TransportRequest, header_get

USER_AGENT = "kirivers-client-python/0.1.0"
DEFAULT_CAPABILITIES = ["full_package"]
_JSON_OK = {200, 201, 202}
_EMPTY_OK = {204, 304}
_BYTES_OK = {200, 206}


def _quote(part: str) -> str:
    return quote(str(part), safe="")


def _header_map(headers: Mapping[str, str] | None) -> dict[str, str]:
    return {k: v for k, v in (headers or {}).items()}


@dataclass
class Config:
    """Caller-supplied identity and plane URL. The SDK never invents `device_id`."""

    base_url: str
    project_ref: str
    project_token: str | None = None
    channel_token: str | None = None
    timeout: float = 30.0

    def __post_init__(self) -> None:
        self.base_url = (self.base_url or "").rstrip("/")
        self.project_ref = (self.project_ref or "").strip()
        if not self.base_url:
            raise ConfigError("base_url is required")
        if not self.project_ref:
            raise ConfigError("project_ref is required")


class Client:
    """Typed access to every native JSON operation (store feeds and leftover paths excluded)."""

    def __init__(self, config: Config, transport: Transport | None = None) -> None:
        self.config = config
        self._owns_transport = transport is None
        self.transport = transport or RequestsTransport(timeout=config.timeout)

    def close(self) -> None:
        close = getattr(self.transport, "close", None)
        if self._owns_transport and callable(close):
            close()

    def __enter__(self) -> Client:
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()

    def _auth_headers(self, extra: dict[str, str] | None = None) -> dict[str, str]:
        headers = {"User-Agent": USER_AGENT, "Accept": "application/json"}
        token = self.config.project_token
        if token:
            headers["Authorization"] = f"Bearer {token}"
            headers["X-Project-Token"] = token
        if self.config.channel_token:
            headers["X-Channel-Token"] = self.config.channel_token
        if extra:
            headers.update(extra)
        return headers

    def _same_origin(self, url: str) -> bool:
        parsed = urlparse(url)
        if not parsed.netloc:
            return True
        base = urlparse(self.config.base_url)
        return parsed.scheme.lower() == base.scheme.lower() and parsed.netloc.lower() == base.netloc.lower()

    def _headers_for_url(self, url: str, extra: dict[str, str] | None = None) -> dict[str, str]:
        """Send project/channel tokens only to the client-plane origin.

        Check `package_url` may be an absolute public object URL (S3/CDN). Those
        hosts must not receive `Authorization` / `X-Project-Token`.
        """
        if self._same_origin(url):
            return self._auth_headers(extra)
        headers = {"User-Agent": USER_AGENT}
        if extra:
            headers.update(extra)
        for key in list(headers):
            if key.lower() in {"authorization", "x-project-token", "x-channel-token"}:
                del headers[key]
        return headers

    def _project_url(self, *parts: str, query: Mapping[str, Any] | None = None) -> str:
        segs = "/".join(_quote(p) for p in parts if p is not None)
        path = f"/api/v1/projects/{_quote(self.config.project_ref)}"
        if segs:
            path = f"{path}/{segs}"
        url = self.config.base_url + path
        if query:
            items = []
            for key, value in query.items():
                if value is None:
                    continue
                if isinstance(value, bool):
                    items.append((key, "true" if value else "false"))
                else:
                    items.append((key, str(value)))
            if items:
                url = f"{url}?{urlencode(items)}"
        return url

    def resolve_url(self, maybe_relative: str) -> str:
        """Join a check/diff `package_url` with `base_url`, keeping `exp`/`sig` query."""
        raw = (maybe_relative or "").strip()
        if not raw:
            raise ConfigError("empty package url")
        if raw.startswith("http://") or raw.startswith("https://"):
            return raw
        return urljoin(self.config.base_url + "/", raw)

    def _send(
        self,
        method: str,
        url: str,
        *,
        headers: dict[str, str] | None = None,
        body: bytes | None = None,
        accept: set[int] | None = None,
    ) -> tuple[int, dict[str, str], bytes]:
        req = TransportRequest(method=method.upper(), url=url, headers=headers or {}, body=body)
        resp = self.transport.request(req)
        allowed = accept or (_JSON_OK | _EMPTY_OK | _BYTES_OK)
        if resp.status not in allowed:
            self._raise_api_error(resp.status, resp.headers, resp.body)
        return resp.status, resp.headers, resp.body

    def _raise_api_error(self, status: int, headers: Mapping[str, str], body: bytes) -> None:
        code = f"HTTP_{status}"
        message = (body or b"").decode("utf-8", "replace")[:800] or http_reason(status)
        details: Any = None
        raw: dict[str, Any] | None = None
        try:
            payload = json.loads(body.decode("utf-8")) if body else None
        except (UnicodeDecodeError, json.JSONDecodeError):
            payload = None
        if isinstance(payload, dict):
            raw = payload
            err = payload.get("error")
            if isinstance(err, dict):
                code = str(err.get("code") or code)
                message = str(err.get("message") or message)
                details = err.get("details")
        retry_raw = header_get(headers, "Retry-After")
        retry_after = None
        if retry_raw:
            try:
                retry_after = int(retry_raw.split(",")[0].strip())
            except ValueError:
                retry_after = None
        raise APIError(status, code, message, details=details, retry_after=retry_after, raw=raw)

    def _json_body(self, payload: dict[str, Any]) -> bytes:
        return json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")

    def _parse_json(self, body: bytes) -> dict[str, Any]:
        if not body:
            return {}
        data = json.loads(body.decode("utf-8"))
        if not isinstance(data, dict):
            raise APIError(200, "INVALID_RESPONSE", "expected a JSON object")
        return data

    def health(self) -> Health:
        """GET /api/v1/health — HTTP is always 200; `ready` is DB + storage."""
        url = f"{self.config.base_url}/api/v1/health"
        status, _, body = self._send("GET", url, headers=self._auth_headers(), accept={200})
        return Health.from_json(self._parse_json(body) if status == 200 else {})

    def project(self) -> ProjectPublic:
        """GET /api/v1/projects/{project_ref}"""
        url = self._project_url()
        _, _, body = self._send("GET", url, headers=self._auth_headers(), accept={200})
        return ProjectPublic.from_json(self._parse_json(body))

    def device_report(
        self,
        device_id: str,
        *,
        version: str | None = None,
        os: str | None = None,
        arch: str | None = None,
        channel: str | None = None,
        custom: dict[str, Any] | None = None,
    ) -> DeviceReport:
        """POST .../clients/report — 200 is geo-only; never an admin roster row."""
        payload = omit_empty(
            {
                "device_id": device_id,
                "version": version,
                "os": os,
                "arch": arch,
                "channel": channel,
                "custom": custom,
            }
        )
        url = self._project_url("clients", "report")
        headers = self._auth_headers({"Content-Type": "application/json"})
        _, _, body = self._send("POST", url, headers=headers, body=self._json_body(payload), accept={200})
        return DeviceReport.from_json(self._parse_json(body))

    def check(
        self,
        *,
        current_version: str,
        os: str,
        arch: str,
        channel: str | None = None,
        hw_rev: str | None = None,
        os_version: str | None = None,
        device_id: str | None = None,
        capabilities: list[str] | None = None,
        accepted_delta_algos: list[str] | None = None,
        etag: str | None = None,
    ) -> CheckOutcome:
        """POST .../update/check. Default capabilities are `["full_package"]` (D13).

        HTTP 204 is no update (not an error). HTTP 304 is an ETag hit.
        """
        caps = list(capabilities) if capabilities else list(DEFAULT_CAPABILITIES)
        if not caps:
            caps = list(DEFAULT_CAPABILITIES)
        payload: dict[str, Any] = {
            "current_version": current_version,
            "os": os,
            "arch": arch,
            "capabilities": caps,
        }
        payload.update(
            omit_empty(
                {
                    "channel": channel,
                    "hw_rev": hw_rev,
                    "os_version": os_version,
                    "device_id": device_id,
                }
            )
        )
        if accepted_delta_algos:
            payload["accepted_delta_algos"] = list(accepted_delta_algos)
        extra: dict[str, str] = {"Content-Type": "application/json"}
        if etag:
            extra["If-None-Match"] = etag
        url = self._project_url("update", "check")
        status, headers, body = self._send(
            "POST",
            url,
            headers=self._auth_headers(extra),
            body=self._json_body(payload),
            accept={200, 204, 304},
        )
        etag_out = header_get(headers, "ETag")
        cache = header_get(headers, "Cache-Control")
        if status == 304:
            return CheckOutcome(status=304, etag=etag_out, not_modified=True, cache_control=cache)
        if status == 204:
            return CheckOutcome(status=204, etag=etag_out, no_update=True, cache_control=cache)
        update = UpdateCheck.from_json(self._parse_json(body))
        return CheckOutcome(status=200, etag=etag_out, update=update, cache_control=cache)

    def changelog(
        self,
        channel: str,
        os: str,
        arch: str,
        *,
        from_version: str | None = None,
        to_version: str | None = None,
        changelog_scope: str | None = None,
        changelog_layout: str | None = None,
        changelog_include_revoked: bool | None = None,
        changelog_include_platform_notes: bool | None = None,
        changelog_locale: str | None = None,
        locale: str | None = None,
        etag: str | None = None,
    ) -> Changelog:
        """GET .../changelog/{channel}/{os}/{arch} — only native changelog body."""
        query = {
            "from_version": from_version,
            "to_version": to_version,
            "changelog_scope": changelog_scope,
            "changelog_layout": changelog_layout,
            "changelog_include_revoked": changelog_include_revoked,
            "changelog_include_platform_notes": changelog_include_platform_notes,
            "changelog_locale": changelog_locale,
            "locale": locale,
        }
        extra: dict[str, str] = {}
        if etag:
            extra["If-None-Match"] = etag
        url = self._project_url("changelog", channel, os, arch, query=query)
        status, headers, body = self._send(
            "GET", url, headers=self._auth_headers(extra), accept={200, 304}
        )
        etag_out = header_get(headers, "ETag")
        if status == 304:
            return Changelog(etag=etag_out, not_modified=True, status=304)
        result = Changelog.from_json(self._parse_json(body))
        result.etag = etag_out
        result.status = 200
        return result

    def integrity(
        self,
        version: str,
        *,
        os: str,
        arch: str,
        hash_algo: str | None = None,
        compact: bool | None = None,
        include_file_urls: bool | None = None,
        hw_rev: str | None = None,
        channel: str | None = None,
        etag: str | None = None,
    ) -> Integrity:
        """GET .../versions/{version}/integrity — all files, no cursor."""
        query = {
            "os": os,
            "arch": arch,
            "hash_algo": hash_algo,
            "compact": compact,
            "include_file_urls": include_file_urls,
            "hw_rev": hw_rev,
            "channel": channel,
        }
        extra: dict[str, str] = {}
        if etag:
            extra["If-None-Match"] = etag
        url = self._project_url("versions", version, "integrity", query=query)
        status, headers, body = self._send(
            "GET", url, headers=self._auth_headers(extra), accept={200, 304}
        )
        etag_out = header_get(headers, "ETag")
        if status == 304:
            return Integrity(etag=etag_out, not_modified=True, status=304)
        result = Integrity.from_json(self._parse_json(body))
        result.etag = etag_out
        result.status = 200
        return result

    def diff(
        self,
        *,
        source_version: str,
        target_version: str,
        os: str,
        arch: str,
        channel: str | None = None,
        hw_rev: str | None = None,
        device_id: str | None = None,
        local_sha256: str | None = None,
        capabilities: list[str] | None = None,
        accepted_delta_algos: list[str] | None = None,
        prefer_full: bool | None = None,
    ) -> Diff:
        """POST .../update/diff — lookup only; `local_sha256` lives here, never on check."""
        payload = omit_empty(
            {
                "source_version": source_version,
                "target_version": target_version,
                "os": os,
                "arch": arch,
                "channel": channel,
                "hw_rev": hw_rev,
                "device_id": device_id,
                "local_sha256": local_sha256,
                "capabilities": capabilities,
                "accepted_delta_algos": accepted_delta_algos,
                "prefer_full": prefer_full,
            }
        )
        url = self._project_url("update", "diff")
        headers = self._auth_headers({"Content-Type": "application/json"})
        _, _, body = self._send("POST", url, headers=headers, body=self._json_body(payload), accept={200})
        return Diff.from_json(self._parse_json(body))

    def _pack_json(
        self,
        *,
        source_version: str,
        target_version: str,
        os: str,
        arch: str,
        needed_paths: list[str] | None = None,
        channel: str | None = None,
        hw_rev: str | None = None,
        device_id: str | None = None,
    ) -> bytes:
        payload: dict[str, Any] = {
            "source_version": source_version,
            "target_version": target_version,
            "os": os,
            "arch": arch,
        }
        payload.update(
            omit_empty(
                {
                    "channel": channel,
                    "hw_rev": hw_rev,
                    "device_id": device_id,
                }
            )
        )
        # Empty needed_paths is a valid fileset (nothing to fetch); do not omit it.
        if needed_paths is not None:
            payload["needed_paths"] = list(needed_paths)
        return self._json_body(payload)

    def pack(
        self,
        *,
        source_version: str,
        target_version: str,
        os: str,
        arch: str,
        needed_paths: list[str] | None = None,
        channel: str | None = None,
        hw_rev: str | None = None,
        device_id: str | None = None,
        body: bytes | None = None,
    ) -> Pack:
        """POST .../update/pack once. 202 `pending` has no `package_url`. Poll with the same JSON."""
        if body is None:
            body = self._pack_json(
                source_version=source_version,
                target_version=target_version,
                os=os,
                arch=arch,
                needed_paths=needed_paths,
                channel=channel,
                hw_rev=hw_rev,
                device_id=device_id,
            )
        url = self._project_url("update", "pack")
        headers = self._auth_headers({"Content-Type": "application/json"})
        status, _, raw = self._send("POST", url, headers=headers, body=body, accept={200, 202})
        result = Pack.from_json(self._parse_json(raw))
        result.http_status = status
        return result

    def pack_until_ready(
        self,
        *,
        source_version: str,
        target_version: str,
        os: str,
        arch: str,
        needed_paths: list[str] | None = None,
        channel: str | None = None,
        hw_rev: str | None = None,
        device_id: str | None = None,
        deadline_s: float = 120.0,
        initial_wait_s: float = 1.0,
        max_wait_s: float = 15.0,
        sleep=None,
        clock=None,
    ) -> Pack:
        """POST the identical pack JSON until `ready` / `full_package` or the deadline."""
        import time as time_mod

        sleep_fn = time_mod.sleep if sleep is None else sleep
        now = time_mod.monotonic if clock is None else clock
        body = self._pack_json(
            source_version=source_version,
            target_version=target_version,
            os=os,
            arch=arch,
            needed_paths=needed_paths,
            channel=channel,
            hw_rev=hw_rev,
            device_id=device_id,
        )
        deadline = now() + deadline_s
        delay = initial_wait_s
        last: Pack | None = None
        while now() < deadline:
            last = self.pack(
                source_version=source_version,
                target_version=target_version,
                os=os,
                arch=arch,
                body=body,
            )
            if last.status in ("ready", "full_package"):
                return last
            if last.http_status != 202 and last.status != "pending":
                return last
            remaining = deadline - now()
            if remaining <= 0:
                break
            sleep_fn(min(delay, remaining))
            delay = min(delay * 2.0, max_wait_s)
        if last is None:
            raise APIError(408, "PACK_TIMEOUT", "pack polling produced no response")
        if last.status == "pending" or last.http_status == 202:
            raise APIError(408, "PACK_TIMEOUT", "pack still pending after deadline")
        return last

    def download(
        self,
        ref: str,
        *,
        byte_range: str | None = None,
        exp: str | None = None,
        sig: str | None = None,
        hw_rev: str | None = None,
    ) -> BinaryResult:
        """GET .../packages/{ref} (content SHA-256). Keeps optional `exp`/`sig`."""
        query = {"exp": exp, "sig": sig, "hw_rev": hw_rev}
        url = self._project_url("packages", ref, query=query)
        return self._get_bytes(url, byte_range=byte_range)

    def head_package(
        self,
        ref: str,
        *,
        exp: str | None = None,
        sig: str | None = None,
        hw_rev: str | None = None,
        byte_range: str | None = None,
    ) -> BinaryResult:
        """HEAD .../packages/{ref}."""
        query = {"exp": exp, "sig": sig, "hw_rev": hw_rev}
        url = self._project_url("packages", ref, query=query)
        return self._head_bytes(url, byte_range=byte_range)

    def download_url(self, package_url: str, *, byte_range: str | None = None) -> BinaryResult:
        """GET a check/diff/pack `package_url`, preserving signed query parameters."""
        return self._get_bytes(self.resolve_url(package_url), byte_range=byte_range)

    def head_url(self, package_url: str, *, byte_range: str | None = None) -> BinaryResult:
        return self._head_bytes(self.resolve_url(package_url), byte_range=byte_range)

    def _get_bytes(self, url: str, *, byte_range: str | None = None) -> BinaryResult:
        extra = {"Accept": "application/octet-stream"}
        if byte_range:
            extra["Range"] = byte_range
        status, headers, body = self._send(
            "GET", url, headers=self._headers_for_url(url, extra), accept={200, 206}
        )
        return BinaryResult(
            status=status,
            body=body,
            headers=_header_map(headers),
            content_type=header_get(headers, "Content-Type"),
            etag=header_get(headers, "ETag"),
        )

    def _head_bytes(self, url: str, *, byte_range: str | None = None) -> BinaryResult:
        extra = {"Accept": "application/octet-stream"}
        if byte_range:
            extra["Range"] = byte_range
        status, headers, body = self._send(
            "HEAD", url, headers=self._headers_for_url(url, extra), accept={200, 206}
        )
        return BinaryResult(
            status=status,
            body=body or b"",
            headers=_header_map(headers),
            content_type=header_get(headers, "Content-Type"),
            etag=header_get(headers, "ETag"),
        )

    def channels(self) -> list[Channel]:
        """GET .../channels"""
        url = self._project_url("channels")
        _, _, body = self._send("GET", url, headers=self._auth_headers(), accept={200})
        data = self._parse_json(body)
        return [Channel.from_json(item) for item in (data.get("channels") or []) if isinstance(item, dict)]

    def matrix(self) -> list[MatrixRow]:
        """GET .../matrix"""
        url = self._project_url("matrix")
        _, _, body = self._send("GET", url, headers=self._auth_headers(), accept={200})
        data = self._parse_json(body)
        return [MatrixRow.from_json(item) for item in (data.get("matrix") or []) if isinstance(item, dict)]

    def languages(self) -> list[Language]:
        """GET .../languages — `{languages: []}`."""
        url = self._project_url("languages")
        _, _, body = self._send("GET", url, headers=self._auth_headers(), accept={200})
        data = self._parse_json(body)
        return [Language.from_json(item) for item in (data.get("languages") or []) if isinstance(item, dict)]

    def announcements(
        self,
        *,
        version: str | None = None,
        os: str | None = None,
        arch: str | None = None,
        locale: str | None = None,
        accept_language: str | None = None,
        etag: str | None = None,
    ) -> EnvelopeResult:
        """GET .../announcements — independent of check."""
        query = {"version": version, "os": os, "arch": arch, "locale": locale}
        extra: dict[str, str] = {}
        if accept_language:
            extra["Accept-Language"] = accept_language
        if etag:
            extra["If-None-Match"] = etag
        url = self._project_url("announcements", query=query)
        status, headers, body = self._send(
            "GET", url, headers=self._auth_headers(extra), accept={200, 304}
        )
        etag_out = header_get(headers, "ETag")
        if status == 304:
            return EnvelopeResult(status=304, etag=etag_out, not_modified=True, body={})
        data = self._parse_json(body)
        items = [Announcement.from_json(item) for item in (data.get("announcements") or []) if isinstance(item, dict)]
        return EnvelopeResult(
            status=200,
            etag=etag_out,
            not_modified=False,
            body={"announcements": items},
        )

    def report_telemetry(
        self,
        *,
        os: str,
        arch: str,
        channel: str,
        from_version: str,
        to_version: str,
        status: str,
        device_id: str | None = None,
        diff_mode: str | None = None,
        error_code: str | None = None,
        error_message: str | None = None,
    ) -> dict[str, Any]:
        """POST .../telemetry/report — success is 202; callers must not block apply on failure."""
        payload = omit_empty(
            {
                "os": os,
                "arch": arch,
                "channel": channel,
                "from_version": from_version,
                "to_version": to_version,
                "status": status,
                "device_id": device_id,
                "diff_mode": diff_mode,
                "error_code": error_code,
                "error_message": error_message,
            }
        )
        url = self._project_url("telemetry", "report")
        headers = self._auth_headers({"Content-Type": "application/json"})
        _, _, body = self._send("POST", url, headers=headers, body=self._json_body(payload), accept={202})
        try:
            return self._parse_json(body)
        except APIError:
            return {"status": "accepted"}

    def media(self, media_id: str, *, byte_range: str | None = None) -> BinaryResult:
        """GET .../media/{id}"""
        url = self._project_url("media", media_id)
        return self._get_bytes(url, byte_range=byte_range)

    def head_media(self, media_id: str, *, byte_range: str | None = None) -> BinaryResult:
        """HEAD .../media/{id}"""
        url = self._project_url("media", media_id)
        return self._head_bytes(url, byte_range=byte_range)


def http_reason(status: int) -> str:
    return {
        400: "bad request",
        401: "unauthorized",
        403: "forbidden",
        404: "not found",
        409: "conflict",
        412: "precondition failed",
        429: "rate limited",
        500: "internal error",
    }.get(status, f"http {status}")
