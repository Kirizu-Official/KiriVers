from __future__ import annotations

import json
from typing import Callable

from kirivers_client.transport import TransportRequest, TransportResponse


class MockTransport:
    """Queue of TransportResponse values; records every request."""

    def __init__(
        self,
        handler: Callable[[TransportRequest], TransportResponse] | None = None,
        responses: list[TransportResponse] | TransportResponse | None = None,
    ) -> None:
        self.calls: list[TransportRequest] = []
        self._handler = handler
        if responses is None:
            self._queue: list[TransportResponse] = []
        elif isinstance(responses, list):
            self._queue = list(responses)
        else:
            self._queue = [responses]

    def request(self, req: TransportRequest) -> TransportResponse:
        self.calls.append(req)
        if self._handler is not None:
            return self._handler(req)
        if not self._queue:
            raise AssertionError(f"no mock response for {req.method} {req.url}")
        if len(self._queue) == 1:
            return self._queue[0]
        return self._queue.pop(0)


def json_response(status: int, payload: dict, extra_headers: dict[str, str] | None = None) -> TransportResponse:
    headers = {"Content-Type": "application/json"}
    if extra_headers:
        headers.update(extra_headers)
    return TransportResponse(status=status, headers=headers, body=json.dumps(payload).encode("utf-8"))


def empty_response(status: int, extra_headers: dict[str, str] | None = None) -> TransportResponse:
    return TransportResponse(status=status, headers=dict(extra_headers or {}), body=b"")
