"""SDK error types. HTTP failures keep the server error envelope."""

from __future__ import annotations

from typing import Any


class KiriVersError(Exception):
    """Base class for SDK errors."""


class APIError(KiriVersError):
    """JSON error envelope `{error:{code,message,details}}` or a non-success status."""

    def __init__(
        self,
        status: int,
        code: str,
        message: str,
        *,
        details: Any = None,
        retry_after: int | None = None,
        raw: dict[str, Any] | None = None,
    ) -> None:
        super().__init__(f"{code}: {message}")
        self.status = status
        self.code = code
        self.message = message
        self.details = details
        self.retry_after = retry_after
        self.raw = raw


class PathError(KiriVersError):
    """Relative path failed NFC / slash / traversal checks."""


class SignatureError(KiriVersError):
    """Check/integrity payload signature did not verify."""


class PatchError(KiriVersError):
    """Delta bytes could not be applied (unknown magic or Patcher failure)."""


class HashMismatchError(KiriVersError):
    """Downloaded bytes did not match the advertised SHA-256."""

    def __init__(self, expected: str, actual: str) -> None:
        super().__init__(f"sha256 mismatch: expected {expected}, got {actual}")
        self.expected = expected
        self.actual = actual


class ConfigError(KiriVersError):
    """Invalid Client / Updater configuration."""
