"""Official KiriVers native client SDK (PyPI: kirivers-client)."""

from __future__ import annotations

from kirivers_client.adapters import (
    ArchiveUnpacker,
    CryptographyVerifier,
    FileStore,
    HashlibHasher,
    Hasher,
    OsReplaceReplacer,
    Patcher,
    PathFileStore,
    Replacer,
    RequestsTransport,
    SignatureVerifier,
    ZipArchiveUnpacker,
    build_check_payload,
    derive_capabilities,
)
from kirivers_client.client import Client, Config
from kirivers_client.delta import detect_delta_algo
from kirivers_client.errors import (
    APIError,
    ConfigError,
    HashMismatchError,
    KiriVersError,
    PatchError,
    PathError,
    SignatureError,
)
from kirivers_client.paths import normalize_path
from kirivers_client.transport import Transport, TransportRequest, TransportResponse
from kirivers_client.updater import PublicKey, UpdateResult, Updater

__version__ = "0.1.0"

__all__ = [
    "APIError",
    "ArchiveUnpacker",
    "Client",
    "Config",
    "ConfigError",
    "CryptographyVerifier",
    "FileStore",
    "HashMismatchError",
    "Hasher",
    "HashlibHasher",
    "KiriVersError",
    "OsReplaceReplacer",
    "PatchError",
    "Patcher",
    "PathError",
    "PathFileStore",
    "PublicKey",
    "Replacer",
    "RequestsTransport",
    "SignatureError",
    "SignatureVerifier",
    "Transport",
    "TransportRequest",
    "TransportResponse",
    "UpdateResult",
    "Updater",
    "ZipArchiveUnpacker",
    "__version__",
    "build_check_payload",
    "derive_capabilities",
    "detect_delta_algo",
    "normalize_path",
]
