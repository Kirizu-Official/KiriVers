"""HTTP Transport adapter. Default implementation uses `requests` (D18)."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Mapping, Protocol

import requests


def header_get(headers: Mapping[str, str], name: str) -> str | None:
    target = name.lower()
    for key, value in headers.items():
        if key.lower() == target:
            return value
    return None


@dataclass
class TransportRequest:
    method: str
    url: str
    headers: dict[str, str] = field(default_factory=dict)
    body: bytes | None = None


@dataclass
class TransportResponse:
    status: int
    headers: dict[str, str]
    body: bytes


class Transport(Protocol):
    """HTTP GET/HEAD/POST with caller headers, body bytes, and query already on the URL.

    Implementations must honor `Range` and must not strip `?exp=&sig=`.
    """

    def request(self, req: TransportRequest) -> TransportResponse: ...


class RequestsTransport:
    """Default Transport: `requests.Session` only — no httpx / aiohttp."""

    def __init__(self, timeout: float = 30.0, session: requests.Session | None = None) -> None:
        self.timeout = timeout
        self._owns_session = session is None
        self._session = session or requests.Session()

    def request(self, req: TransportRequest) -> TransportResponse:
        resp = self._session.request(
            req.method,
            req.url,
            headers=req.headers or None,
            data=req.body,
            timeout=self.timeout,
            allow_redirects=True,
        )
        headers = {k: v for k, v in resp.headers.items()}
        return TransportResponse(status=resp.status_code, headers=headers, body=resp.content or b"")

    def close(self) -> None:
        if self._owns_session:
            self._session.close()
