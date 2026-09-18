"""Wire JSON models using OpenAPI property names (snake_case)."""

from __future__ import annotations

from dataclasses import dataclass, field, fields
from typing import Any


def _from_mapping(cls: type, data: dict[str, Any] | None) -> Any:
    if not data:
        return cls()
    names = {f.name for f in fields(cls)}
    kwargs = {k: v for k, v in data.items() if k in names}
    return cls(**kwargs)


def omit_empty(data: dict[str, Any]) -> dict[str, Any]:
    """Drop None / empty-list / empty-string optional JSON fields."""
    out: dict[str, Any] = {}
    for key, value in data.items():
        if value is None or value == "" or value == []:
            continue
        out[key] = value
    return out


@dataclass
class UpdateCheck:
    has_update: bool = False
    is_mandatory: bool = False
    is_downgrade: bool = False
    reason: str = ""
    compare_engine: str = ""
    version_integer: int | None = None
    version_semver: str | None = None
    target_channel: str = ""
    target_hw_rev: str | None = None
    package_type: str = ""
    root_hash: str = ""
    package_url: str = ""
    file_name: str = ""
    size: int = 0
    sha256: str = ""
    delta_available: bool = False
    delta_algo: str | None = None
    platform_notes: str = ""
    publish_time: str | None = None
    signature: str | None = None
    artifact_signature: str | None = None

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> UpdateCheck:
        return _from_mapping(cls, data)


@dataclass
class CheckOutcome:
    """POST /update/check: 200 body, 204 no update, or 304 ETag hit — none of these are errors."""

    status: int
    etag: str | None = None
    not_modified: bool = False
    no_update: bool = False
    update: UpdateCheck | None = None
    cache_control: str | None = None

    @property
    def has_update(self) -> bool:
        return self.update is not None and self.update.has_update and not self.not_modified and not self.no_update


@dataclass
class DeviceReport:
    ip: str = ""
    country_code: str = ""
    region_code: str = ""
    geo_i18n: dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> DeviceReport:
        obj = _from_mapping(cls, data)
        if obj.geo_i18n is None:
            obj.geo_i18n = {}
        return obj


@dataclass
class ProjectPublic:
    uuid: str = ""
    slug: str = ""
    compare_engine: str = ""
    default_locale: str = ""
    device_id_policy: str = ""
    force_https: bool = False
    require_client_token: bool = False
    storage_visibility: str = ""
    minimum_supported_version: str | None = None
    created_at: str = ""
    updated_at: str = ""

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> ProjectPublic:
        return _from_mapping(cls, data)


@dataclass
class Channel:
    slug: str = ""
    name: str = ""
    stability_rank: int = 0

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> Channel:
        return _from_mapping(cls, data)


@dataclass
class MatrixRow:
    os: str = ""
    arch: str = ""
    package_type: str = ""

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> MatrixRow:
        return _from_mapping(cls, data)


@dataclass
class Language:
    code: str = ""
    display_name: str = ""
    is_default: bool = False
    sort_order: int = 0

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> Language:
        return _from_mapping(cls, data)


@dataclass
class Announcement:
    id: str = ""
    title: str = ""
    subtitle: str = ""
    markdown: str = ""
    locale: str = ""
    starts_at: str | None = None
    ends_at: str | None = None

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> Announcement:
        return _from_mapping(cls, data)


@dataclass
class ChangelogVersion:
    channel: str = ""
    status: str = ""
    changelog: str = ""
    had_artifact_for_request_platform: bool = False
    version_integer: int | None = None
    version_semver: str | None = None
    title: str = ""
    platform_notes: str = ""

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> ChangelogVersion:
        return _from_mapping(cls, data)


@dataclass
class Changelog:
    changelog: str = ""
    changelog_versions: list[ChangelogVersion] = field(default_factory=list)
    etag: str | None = None
    not_modified: bool = False
    status: int = 200

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> Changelog:
        versions = [
            ChangelogVersion.from_json(item)
            for item in (data.get("changelog_versions") or [])
            if isinstance(item, dict)
        ]
        return cls(
            changelog=data.get("changelog") or "",
            changelog_versions=versions,
        )


@dataclass
class IntegrityFile:
    path: str = ""
    size: int = 0
    install_policy: str = "OVERWRITE"
    integrity_check: bool = True
    sha256: str = ""
    md5: str = ""
    url: str | None = None

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> IntegrityFile:
        return _from_mapping(cls, data)


@dataclass
class Integrity:
    version_integer: int | None = None
    version_semver: str | None = None
    channel: str = ""
    package_type: str = ""
    root_hash: str = ""
    full_package_url: str = ""
    file_name: str = ""
    size: int = 0
    sha256: str = ""
    files: list[IntegrityFile] = field(default_factory=list)
    signature: str | None = None
    volumes: list[dict[str, Any]] | None = None
    etag: str | None = None
    not_modified: bool = False
    status: int = 200

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> Integrity:
        files = [
            IntegrityFile.from_json(item)
            for item in (data.get("files") or [])
            if isinstance(item, dict)
        ]
        obj = _from_mapping(cls, data)
        obj.files = files
        return obj


@dataclass
class DiffFile:
    path: str = ""
    size: int = 0
    sha256: str = ""
    md5: str = ""
    url: str | None = None

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> DiffFile:
        return _from_mapping(cls, data)


@dataclass
class Diff:
    diff_mode: str = ""
    root_hash: str = ""
    version_integer: int | None = None
    version_semver: str | None = None
    channel: str = ""
    compare_engine: str = ""
    package_url: str = ""
    file_name: str = ""
    size: int = 0
    sha256: str = ""
    signature: str | None = None
    delta_algo: str | None = None
    files: list[DiffFile] = field(default_factory=list)
    deleted_paths: list[str] = field(default_factory=list)
    invalid_paths: list[str] = field(default_factory=list)

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> Diff:
        files = [
            DiffFile.from_json(item)
            for item in (data.get("files") or [])
            if isinstance(item, dict)
        ]
        obj = _from_mapping(cls, data)
        obj.files = files
        obj.deleted_paths = list(data.get("deleted_paths") or [])
        obj.invalid_paths = list(data.get("invalid_paths") or [])
        return obj


@dataclass
class PackFile:
    path: str = ""
    size: int = 0
    sha256: str = ""
    install_policy: str = ""
    integrity_check: bool = False

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> PackFile:
        return _from_mapping(cls, data)


@dataclass
class Pack:
    status: str = ""
    package_url: str = ""
    sha256: str = ""
    size: int = 0
    file_name: str = ""
    root_hash: str = ""
    signature: str | None = None
    compression: str = ""
    diff_mode: str = ""
    channel: str = ""
    compare_engine: str = ""
    version_integer: int | None = None
    version_semver: str | None = None
    files: list[PackFile] = field(default_factory=list)
    deleted_paths: list[str] = field(default_factory=list)
    invalid_paths: list[str] = field(default_factory=list)
    http_status: int = 200

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> Pack:
        files = [
            PackFile.from_json(item)
            for item in (data.get("files") or [])
            if isinstance(item, dict)
        ]
        obj = _from_mapping(cls, data)
        obj.files = files
        obj.deleted_paths = list(data.get("deleted_paths") or [])
        obj.invalid_paths = list(data.get("invalid_paths") or [])
        return obj


@dataclass
class Health:
    status: str = ""
    ready: bool = False

    @classmethod
    def from_json(cls, data: dict[str, Any]) -> Health:
        return _from_mapping(cls, data)


@dataclass
class BinaryResult:
    """GET/HEAD bytes or headers for packages and media."""

    status: int
    body: bytes
    headers: dict[str, str]
    content_type: str | None = None
    etag: str | None = None


@dataclass
class EnvelopeResult:
    """GET that may 304: announcements / similar lists."""

    status: int
    etag: str | None
    not_modified: bool
    body: dict[str, Any]
