"""Injectable adapters. Defaults follow D16/D18; Patcher has no default."""

from __future__ import annotations

import hashlib
import os
import zipfile
from io import BytesIO
from pathlib import Path
from typing import Mapping, Protocol, Sequence

from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ed25519, padding, rsa

from kirivers_client.delta import detect_delta_algo
from kirivers_client.errors import PatchError, PathError, SignatureError
from kirivers_client.paths import normalize_path
from kirivers_client.transport import RequestsTransport, Transport, TransportRequest, TransportResponse

__all__ = [
    "ArchiveUnpacker",
    "CryptographyVerifier",
    "FileStore",
    "HashlibHasher",
    "Hasher",
    "OsReplaceReplacer",
    "Patcher",
    "PathFileStore",
    "Replacer",
    "RequestsTransport",
    "SignatureVerifier",
    "Transport",
    "TransportRequest",
    "TransportResponse",
    "ZipArchiveUnpacker",
    "build_check_payload",
]


def build_check_payload(
    version_integer: str,
    version_semver: str,
    root_hash: str,
    package_url: str,
    size: str,
    sha256_hex: str,
) -> str:
    """Same layout as `pkg/signature.BuildCheckPayload` (empty strings hold place)."""
    return "\n".join(
        [version_integer, version_semver, root_hash, package_url, size, sha256_hex]
    )


def payload_from_check_fields(
    *,
    version_integer: int | None,
    version_semver: str | None,
    root_hash: str | None,
    package_url: str | None,
    size: int | None,
    sha256_hex: str | None,
) -> str:
    integer = "" if version_integer is None else str(int(version_integer))
    semver = version_semver or ""
    root = root_hash or ""
    url = package_url or ""
    size_s = "" if size is None else str(int(size))
    digest = sha256_hex or ""
    return build_check_payload(integer, semver, root, url, size_s, digest)


class Hasher(Protocol):
    def sha256_hex(self, data: bytes) -> str: ...

    def md5_hex(self, data: bytes) -> str: ...

    def sha256_file(self, path: str | Path) -> str: ...


class HashlibHasher:
    """SHA-256 / MD5 via stdlib `hashlib`."""

    def sha256_hex(self, data: bytes) -> str:
        return hashlib.sha256(data).hexdigest()

    def md5_hex(self, data: bytes) -> str:
        return hashlib.md5(data, usedforsecurity=False).hexdigest()

    def sha256_file(self, path: str | Path) -> str:
        digest = hashlib.sha256()
        with open(path, "rb") as fh:
            for chunk in iter(lambda: fh.read(1024 * 1024), b""):
                digest.update(chunk)
        return digest.hexdigest()


class FileStore(Protocol):
    def can_write_individual_files(self) -> bool: ...

    def exists(self, relative_path: str) -> bool: ...

    def read_bytes(self, relative_path: str) -> bytes: ...

    def write_bytes(self, relative_path: str, data: bytes) -> None: ...

    def sha256_hex(self, relative_path: str) -> str: ...


class PathFileStore:
    """Install-tree FileStore: pathlib/os + NFC paths. Individual files can be written."""

    def __init__(self, root: str | os.PathLike[str] | None = None, hasher: Hasher | None = None) -> None:
        self.root = Path(root) if root is not None else None
        self._hasher = hasher or HashlibHasher()

    def can_write_individual_files(self) -> bool:
        # D13: do not advertise file_list until an install root can accept files.
        return self.root is not None

    def resolve(self, relative_path: str) -> Path:
        norm = normalize_path(relative_path)
        if self.root is None:
            raise PathError("FileStore root is not set")
        return self.root.joinpath(*norm.split("/"))

    def exists(self, relative_path: str) -> bool:
        try:
            return self.resolve(relative_path).is_file()
        except PathError:
            return False

    def read_bytes(self, relative_path: str) -> bytes:
        return self.resolve(relative_path).read_bytes()

    def write_bytes(self, relative_path: str, data: bytes) -> None:
        path = self.resolve(relative_path)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)

    def sha256_hex(self, relative_path: str) -> str:
        return self._hasher.sha256_file(self.resolve(relative_path))


class Replacer(Protocol):
    def replace(self, source: str, destination: str) -> None: ...


class OsReplaceReplacer:
    """Atomic `os.replace`. If the destination is busy, the OS error is raised for the caller."""

    def replace(self, source: str, destination: str) -> None:
        dest = Path(destination)
        dest.parent.mkdir(parents=True, exist_ok=True)
        os.replace(source, destination)


class Patcher(Protocol):
    def supported_algos(self) -> Sequence[str]: ...

    def apply(self, old: bytes, delta: bytes) -> bytes: ...


class ArchiveUnpacker(Protocol):
    def unpack(self, archive: bytes, dest_dir: str, files: Sequence[Mapping[str, object]]) -> None: ...


class ZipArchiveUnpacker:
    """Native zip: members are content hashes; destination names come from `files[].path`."""

    def unpack(self, archive: bytes, dest_dir: str, files: Sequence[Mapping[str, object]]) -> None:
        root = Path(dest_dir)
        root.mkdir(parents=True, exist_ok=True)
        with zipfile.ZipFile(BytesIO(archive)) as zf:
            by_name = {info.filename.replace("\\", "/"): info for info in zf.infolist() if not info.is_dir()}
            for entry in files:
                rel = str(entry.get("path") or "")
                sha = str(entry.get("sha256") or "").lower()
                dest_rel = normalize_path(rel)
                dest = root.joinpath(*dest_rel.split("/"))
                dest.parent.mkdir(parents=True, exist_ok=True)
                info = by_name.get(sha) or by_name.get(dest_rel) or by_name.get(rel.replace("\\", "/"))
                if info is None:
                    # Some packs store `<sha256>.<ext>`; match by hash prefix.
                    info = next((item for name, item in by_name.items() if name.startswith(sha)), None)
                if info is None:
                    raise FileNotFoundError(f"zip member not found for {dest_rel} ({sha})")
                dest.write_bytes(zf.read(info))


class SignatureVerifier(Protocol):
    def verify(self, algo: str, public_key_pem: str, payload: str, signature_b64: str) -> None: ...


class CryptographyVerifier:
    """Ed25519 and RSA-SHA256 (PKCS#1 v1.5) over the check payload; standard base64."""

    def verify(self, algo: str, public_key_pem: str, payload: str, signature_b64: str) -> None:
        import base64

        try:
            signature = base64.b64decode(signature_b64)
        except Exception as exc:  # noqa: BLE001 — surface as SignatureError
            raise SignatureError("invalid signature base64") from exc
        try:
            key = serialization.load_pem_public_key(public_key_pem.encode("utf-8"))
        except Exception as exc:  # noqa: BLE001
            raise SignatureError("invalid public key pem") from exc
        data = payload.encode("utf-8")
        name = (algo or "").strip().lower()
        try:
            if name == "ed25519":
                if not isinstance(key, ed25519.Ed25519PublicKey):
                    raise SignatureError("public key is not Ed25519")
                key.verify(signature, data)
                return
            if name in ("rsa-sha256", "rsa_sha256"):
                if not isinstance(key, rsa.RSAPublicKey):
                    raise SignatureError("public key is not RSA")
                key.verify(signature, data, padding.PKCS1v15(), hashes.SHA256())
                return
        except InvalidSignature as exc:
            raise SignatureError("signature mismatch") from exc
        raise SignatureError(f"unsupported signature algorithm {algo!r}")


def derive_capabilities(
    *,
    patcher: Patcher | None,
    unpacker: ArchiveUnpacker | None,
    file_store: FileStore | None,
) -> tuple[list[str], list[str]]:
    """D13: never advertise a capability the live adapters cannot perform."""
    capabilities = ["full_package"]
    algos: list[str] = []
    if unpacker is not None:
        capabilities.append("patch_package")
    if file_store is not None and file_store.can_write_individual_files():
        capabilities.append("file_list")
    if patcher is not None:
        algos = [str(item).strip() for item in patcher.supported_algos() if str(item).strip()]
        if algos:
            capabilities.append("binary_delta")
    return capabilities, algos


def ensure_patcher_magic(patcher: Patcher, delta: bytes) -> str:
    """Reject unknown magic before apply; Patcher still must not cross-decode."""
    algo = detect_delta_algo(delta)
    supported = {str(item) for item in patcher.supported_algos()}
    if algo not in supported:
        raise PatchError(f"patcher does not support {algo}")
    return algo
