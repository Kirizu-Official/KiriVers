"""High-level Update orchestration. Missing Replacer still stages verified bytes."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any, Sequence

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
    SignatureVerifier,
    ZipArchiveUnpacker,
    derive_capabilities,
    ensure_patcher_magic,
    payload_from_check_fields,
)
from kirivers_client.client import Client
from kirivers_client.errors import APIError, HashMismatchError, PatchError, SignatureError
from kirivers_client.models import CheckOutcome, Integrity, Pack, UpdateCheck
from kirivers_client.paths import normalize_path, unique_needed_paths

_MISSING = object()


@dataclass
class PublicKey:
    algo: str
    pem: str


@dataclass
class UpdateResult:
    outcome: str
    check: CheckOutcome
    staged_path: str | None = None
    sha256: str | None = None
    applied: bool = False
    diff_mode: str | None = None
    changelog: Any = None


class Updater:
    """Check → download → verify → optional patch/unpack/replace → telemetry.

    Default adapters: FileStore, Hasher, zip ArchiveUnpacker, cryptography verifier,
    `os.replace` Replacer. Patcher is interface-only. `run(apply=False)` never
    claims a one-click install.
    """

    def __init__(
        self,
        client: Client,
        *,
        file_store: Any = _MISSING,
        hasher: Hasher | None = None,
        archive_unpacker: Any = _MISSING,
        signature_verifier: SignatureVerifier | None = None,
        patcher: Patcher | None = None,
        replacer: Any = _MISSING,
        public_keys: Sequence[PublicKey] | None = None,
        pack_deadline_s: float = 120.0,
        signing_algo: str | None = None,
    ) -> None:
        self.client = client
        self.file_store: FileStore | None = PathFileStore() if file_store is _MISSING else file_store
        self.hasher = hasher or HashlibHasher()
        self.unpacker: ArchiveUnpacker | None = (
            ZipArchiveUnpacker() if archive_unpacker is _MISSING else archive_unpacker
        )
        self.verifier = signature_verifier or CryptographyVerifier()
        self.patcher = patcher
        self.replacer: Replacer | None = OsReplaceReplacer() if replacer is _MISSING else replacer
        self.public_keys = list(public_keys or [])
        self.pack_deadline_s = pack_deadline_s
        self.signing_algo = signing_algo

    def capabilities(self) -> tuple[list[str], list[str]]:
        return derive_capabilities(
            patcher=self.patcher,
            unpacker=self.unpacker,
            file_store=self.file_store,
        )

    def run(
        self,
        *,
        current_version: str,
        os: str,
        arch: str,
        channel: str | None = None,
        device_id: str | None = None,
        os_version: str | None = None,
        hw_rev: str | None = None,
        custom: dict[str, Any] | None = None,
        report_device: bool = False,
        fetch_changelog: bool = False,
        etag: str | None = None,
        dest_path: str | Path | None = None,
        current_file: str | Path | None = None,
        install_dir: str | Path | None = None,
        apply: bool = False,
        sleep=None,
    ) -> UpdateResult:
        if report_device and device_id:
            self.client.device_report(
                device_id,
                version=current_version,
                os=os,
                arch=arch,
                channel=channel,
                custom=custom,
            )
        if (
            install_dir is not None
            and isinstance(self.file_store, PathFileStore)
            and self.file_store.root is None
        ):
            self.file_store.root = Path(install_dir)
        caps, algos = self.capabilities()
        check = self.client.check(
            current_version=current_version,
            os=os,
            arch=arch,
            channel=channel,
            hw_rev=hw_rev,
            os_version=os_version,
            device_id=device_id,
            capabilities=caps,
            accepted_delta_algos=algos or None,
            etag=etag,
        )
        changelog = None
        if fetch_changelog and channel:
            try:
                changelog = self.client.changelog(channel, os, arch, from_version=current_version)
            except Exception:
                changelog = None
        if check.not_modified or check.no_update or check.update is None:
            return UpdateResult(
                outcome="not_modified" if check.not_modified else "no_update",
                check=check,
                changelog=changelog,
            )
        update = check.update
        dest = Path(dest_path) if dest_path is not None else Path(update.file_name or "package.bin")
        dest.parent.mkdir(parents=True, exist_ok=True)

        telemetry_kwargs = {
            "os": os,
            "arch": arch,
            "channel": channel or update.target_channel or "stable",
            "from_version": current_version,
            "to_version": update.version_semver or update.version_integer or current_version,
            "device_id": device_id,
        }
        try:
            self._telemetry("downloading", **telemetry_kwargs)
            staged, digest, mode = self._obtain(
                update,
                current_version=current_version,
                os=os,
                arch=arch,
                channel=channel,
                device_id=device_id,
                hw_rev=hw_rev,
                dest=dest,
                current_file=current_file,
                install_dir=install_dir,
                sleep=sleep,
            )
            self._verify_signature(update)
            applied = False
            if apply and self.replacer is not None:
                self._telemetry("applying", diff_mode=mode, **telemetry_kwargs)
                self.replacer.replace(str(staged), str(dest))
                applied = True
                staged_out = str(dest)
            else:
                staged_out = str(staged)
            self._telemetry("installed", diff_mode=mode, **telemetry_kwargs)
            return UpdateResult(
                outcome="applied" if applied else "downloaded",
                check=check,
                staged_path=staged_out,
                sha256=digest,
                applied=applied,
                diff_mode=mode,
                changelog=changelog,
            )
        except Exception as exc:
            self._telemetry(
                "failed",
                diff_mode=None,
                error_code=type(exc).__name__,
                error_message=str(exc)[:200],
                **telemetry_kwargs,
            )
            raise

    def _telemetry(self, status: str, **kwargs: Any) -> None:
        try:
            to_version = kwargs.get("to_version")
            self.client.report_telemetry(
                os=str(kwargs.get("os") or ""),
                arch=str(kwargs.get("arch") or ""),
                channel=str(kwargs.get("channel") or "stable"),
                from_version=str(kwargs.get("from_version") or ""),
                to_version="" if to_version is None else str(to_version),
                status=status,
                device_id=kwargs.get("device_id"),
                diff_mode=kwargs.get("diff_mode"),
                error_code=kwargs.get("error_code"),
                error_message=kwargs.get("error_message"),
            )
        except Exception:
            # Telemetry must never fail Check/download/apply (parent D15 / telemetry-privacy).
            return

    def _obtain(
        self,
        update: UpdateCheck,
        *,
        current_version: str,
        os: str,
        arch: str,
        channel: str | None,
        device_id: str | None,
        hw_rev: str | None,
        dest: Path,
        current_file: str | Path | None,
        install_dir: str | Path | None,
        sleep,
    ) -> tuple[Path, str, str]:
        if (
            update.package_type == "single_file"
            and update.delta_available
            and not update.is_downgrade
            and self.patcher is not None
            and current_file is not None
        ):
            try:
                return self._via_delta(
                    update,
                    current_version=current_version,
                    os=os,
                    arch=arch,
                    channel=channel,
                    device_id=device_id,
                    hw_rev=hw_rev,
                    dest=dest,
                    current_file=Path(current_file),
                )
            except (PatchError, APIError, HashMismatchError):
                pass
        if update.package_type == "multi_file" and self.file_store is not None and install_dir is not None:
            try:
                return self._via_pack(
                    update,
                    current_version=current_version,
                    os=os,
                    arch=arch,
                    channel=channel,
                    device_id=device_id,
                    hw_rev=hw_rev,
                    dest=dest,
                    install_dir=Path(install_dir),
                    sleep=sleep,
                )
            except APIError:
                pass
        return self._via_full(update, dest)

    def _via_full(self, update: UpdateCheck, dest: Path) -> tuple[Path, str, str]:
        blob = self.client.download_url(update.package_url).body
        digest = self.hasher.sha256_hex(blob)
        expected = (update.sha256 or "").lower()
        if expected and digest != expected:
            raise HashMismatchError(expected, digest)
        dest.write_bytes(blob)
        return dest, digest, "full_package"

    def _via_delta(
        self,
        update: UpdateCheck,
        *,
        current_version: str,
        os: str,
        arch: str,
        channel: str | None,
        device_id: str | None,
        hw_rev: str | None,
        dest: Path,
        current_file: Path,
    ) -> tuple[Path, str, str]:
        assert self.patcher is not None
        local = self.hasher.sha256_file(current_file)
        caps, algos = self.capabilities()
        diff = self.client.diff(
            source_version=current_version,
            target_version=update.version_semver or str(update.version_integer or ""),
            os=os,
            arch=arch,
            channel=channel or update.target_channel,
            hw_rev=hw_rev,
            device_id=device_id,
            local_sha256=local,
            capabilities=caps,
            accepted_delta_algos=algos or None,
        )
        if diff.diff_mode != "binary_delta" or not diff.package_url:
            raise PatchError("diff did not return binary_delta")
        old = current_file.read_bytes()
        delta = self.client.download_url(diff.package_url).body
        ensure_patcher_magic(self.patcher, delta)
        new_bytes = self.patcher.apply(old, delta)
        digest = self.hasher.sha256_hex(new_bytes)
        expected = (update.sha256 or "").lower()
        if expected and digest != expected:
            raise HashMismatchError(expected, digest)
        dest.write_bytes(new_bytes)
        return dest, digest, "binary_delta"

    def _via_pack(
        self,
        update: UpdateCheck,
        *,
        current_version: str,
        os: str,
        arch: str,
        channel: str | None,
        device_id: str | None,
        hw_rev: str | None,
        dest: Path,
        install_dir: Path,
        sleep,
    ) -> tuple[Path, str, str]:
        store = PathFileStore(install_dir, hasher=self.hasher)
        integrity = self.client.integrity(
            update.version_semver or str(update.version_integer or ""),
            os=os,
            arch=arch,
            hw_rev=hw_rev,
            channel=channel or update.target_channel,
        )
        if integrity.not_modified:
            raise APIError(304, "NOT_MODIFIED", "integrity not modified")
        needed = needed_paths_from_integrity(integrity, store)
        pack = self.client.pack_until_ready(
            source_version=current_version,
            target_version=update.version_semver or str(update.version_integer or ""),
            os=os,
            arch=arch,
            needed_paths=needed,
            channel=channel or update.target_channel,
            hw_rev=hw_rev,
            device_id=device_id,
            deadline_s=self.pack_deadline_s,
            sleep=sleep,
        )
        if pack.status == "full_package":
            return self._via_full(update, dest)
        url = pack.package_url or update.package_url
        blob = self.client.download_url(url).body
        digest = self.hasher.sha256_hex(blob)
        expected = (pack.sha256 or update.sha256 or "").lower()
        if expected and digest != expected:
            raise HashMismatchError(expected, digest)
        dest.write_bytes(blob)
        if self.unpacker is not None and pack.files:
            entries = [{"path": item.path, "sha256": item.sha256} for item in pack.files]
            self.unpacker.unpack(blob, str(install_dir), entries)
        return dest, digest, pack.diff_mode or "patch_package"

    def _verify_signature(self, update: UpdateCheck) -> None:
        if not update.signature or not self.public_keys:
            return
        payload = payload_from_check_fields(
            version_integer=update.version_integer,
            version_semver=update.version_semver,
            root_hash=update.root_hash,
            package_url=update.package_url,
            size=update.size,
            sha256_hex=update.sha256,
        )
        last: Exception | None = None
        for key in self.public_keys:
            algo = self.signing_algo or key.algo
            try:
                self.verifier.verify(algo, key.pem, payload, update.signature)
                return
            except SignatureError as exc:
                last = exc
        raise last or SignatureError("signature mismatch")


def needed_paths_from_integrity(integrity: Integrity, store: FileStore) -> list[str]:
    """KEEP_IF_EXISTS files that already exist locally are omitted from pack `needed_paths`."""
    needed: list[str] = []
    for item in integrity.files:
        try:
            rel = normalize_path(item.path)
        except Exception:
            continue
        policy = (item.install_policy or "OVERWRITE").upper()
        if policy == "KEEP_IF_EXISTS" and store.exists(rel):
            continue
        if store.exists(rel) and item.sha256:
            try:
                if store.sha256_hex(rel).lower() == item.sha256.lower():
                    continue
            except OSError:
                pass
        needed.append(rel)
    return unique_needed_paths(needed)
