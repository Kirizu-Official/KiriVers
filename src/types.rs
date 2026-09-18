use serde::{Deserialize, Serialize};
use serde_json::Value;

fn skip_empty_str(v: &Option<String>) -> bool {
    v.as_ref().map(|s| s.is_empty()).unwrap_or(true)
}

fn skip_empty_vec<T>(v: &[T]) -> bool {
    v.is_empty()
}

/// POST `/update/check` body. Leftover `local_sha256` / `dirty_paths` / changelog fields are not sent.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct CheckRequest {
    pub current_version: String,
    pub os: String,
    pub arch: String,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub channel: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub hw_rev: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub os_version: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub device_id: Option<String>,
    #[serde(default, skip_serializing_if = "skip_empty_vec")]
    pub capabilities: Vec<String>,
    #[serde(default, skip_serializing_if = "skip_empty_vec")]
    pub accepted_delta_algos: Vec<String>,
}

/// Check result. HTTP 204 is not an error. HTTP 304 is an ETag hit.
#[derive(Debug, Clone)]
#[allow(clippy::large_enum_variant)] // `Update` owns the check body; boxing would change the public type.
pub enum CheckOutcome {
    Update {
        body: UpdateCheck,
        etag: Option<String>,
    },
    NoUpdate {
        etag: Option<String>,
    },
    NotModified {
        etag: Option<String>,
    },
}

impl CheckOutcome {
    pub fn etag(&self) -> Option<&str> {
        match self {
            Self::Update { etag, .. } | Self::NoUpdate { etag } | Self::NotModified { etag } => {
                etag.as_deref()
            }
        }
    }
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct UpdateCheck {
    pub has_update: bool,
    pub is_mandatory: bool,
    pub is_downgrade: bool,
    pub reason: String,
    pub compare_engine: String,
    pub version_integer: Option<i64>,
    pub version_semver: Option<String>,
    pub target_channel: String,
    pub target_hw_rev: Option<String>,
    pub package_type: String,
    #[serde(default)]
    pub root_hash: String,
    pub package_url: String,
    pub file_name: String,
    pub size: i64,
    pub sha256: String,
    pub delta_available: bool,
    #[serde(default)]
    pub delta_algo: Option<String>,
    #[serde(default)]
    pub platform_notes: Option<String>,
    #[serde(default)]
    pub publish_time: Option<String>,
    #[serde(default)]
    pub signature: Option<String>,
    #[serde(default)]
    pub artifact_signature: Option<String>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct DeviceReportInput {
    pub device_id: String,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub version: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub os: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub arch: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub channel: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub custom: Option<Value>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct DeviceReportOutput {
    pub ip: String,
    pub country_code: String,
    pub region_code: String,
    #[serde(default)]
    pub geo_i18n: Value,
}

#[derive(Debug, Clone, Default)]
pub struct ChangelogQuery {
    pub channel: String,
    pub os: String,
    pub arch: String,
    pub from_version: Option<String>,
    pub to_version: Option<String>,
    pub changelog_scope: Option<String>,
    pub changelog_layout: Option<String>,
    pub changelog_include_revoked: Option<bool>,
    pub changelog_include_platform_notes: Option<bool>,
    pub changelog_locale: Option<String>,
    pub locale: Option<String>,
    pub if_none_match: Option<String>,
}

#[derive(Debug, Clone)]
pub enum Cached<T> {
    Fresh { value: T, etag: Option<String> },
    NotModified { etag: Option<String> },
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct ChangelogBody {
    #[serde(default)]
    pub changelog: Option<String>,
    #[serde(default)]
    pub changelog_versions: Option<Vec<ChangelogVersion>>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct ChangelogVersion {
    pub channel: String,
    pub status: String,
    pub changelog: String,
    pub had_artifact_for_request_platform: bool,
    pub version_integer: Option<i64>,
    pub version_semver: Option<String>,
    #[serde(default)]
    pub platform_notes: Option<String>,
    #[serde(default)]
    pub title: Option<String>,
}

#[derive(Debug, Clone, Default)]
pub struct IntegrityQuery {
    pub version: String,
    pub os: String,
    pub arch: String,
    pub hash_algo: Option<String>,
    pub compact: Option<bool>,
    pub include_file_urls: Option<bool>,
    pub hw_rev: Option<String>,
    pub channel: Option<String>,
    pub if_none_match: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct IntegrityBody {
    pub version_integer: Option<i64>,
    pub version_semver: Option<String>,
    pub channel: String,
    pub package_type: String,
    #[serde(default)]
    pub root_hash: String,
    pub full_package_url: String,
    pub file_name: String,
    pub size: i64,
    pub sha256: String,
    #[serde(default)]
    pub files: Vec<IntegrityFile>,
    #[serde(default)]
    pub signature: Option<String>,
    #[serde(default)]
    pub volumes: Option<Vec<Volume>>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct IntegrityFile {
    pub path: String,
    pub size: i64,
    #[serde(default)]
    pub sha256: Option<String>,
    #[serde(default)]
    pub md5: Option<String>,
    #[serde(default)]
    pub url: Option<String>,
    #[serde(default)]
    pub install_policy: Option<String>,
    #[serde(default)]
    pub integrity_check: Option<bool>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct Volume {
    #[serde(default)]
    pub sha256: Option<String>,
    #[serde(default)]
    pub size: Option<i64>,
    #[serde(default)]
    pub url: Option<String>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct DiffRequest {
    pub source_version: String,
    pub target_version: String,
    pub os: String,
    pub arch: String,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub channel: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub device_id: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub hw_rev: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub local_sha256: Option<String>,
    #[serde(default, skip_serializing_if = "skip_empty_vec")]
    pub capabilities: Vec<String>,
    #[serde(default, skip_serializing_if = "skip_empty_vec")]
    pub accepted_delta_algos: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub prefer_full: Option<bool>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct DiffResponse {
    pub diff_mode: String,
    #[serde(default)]
    pub root_hash: String,
    pub version_integer: Option<i64>,
    pub version_semver: Option<String>,
    pub channel: String,
    pub compare_engine: String,
    #[serde(default)]
    pub package_url: Option<String>,
    #[serde(default)]
    pub file_name: Option<String>,
    #[serde(default)]
    pub size: Option<i64>,
    #[serde(default)]
    pub sha256: Option<String>,
    #[serde(default)]
    pub signature: Option<String>,
    #[serde(default)]
    pub delta_algo: Option<String>,
    #[serde(default)]
    pub files: Option<Vec<IntegrityFile>>,
    #[serde(default)]
    pub deleted_paths: Option<Vec<String>>,
    #[serde(default)]
    pub invalid_paths: Option<Vec<String>>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PackRequest {
    pub source_version: String,
    pub target_version: String,
    pub os: String,
    pub arch: String,
    #[serde(default, skip_serializing_if = "skip_empty_vec")]
    pub needed_paths: Vec<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub channel: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub device_id: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub hw_rev: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct PackResponse {
    pub status: String,
    #[serde(default)]
    pub diff_mode: Option<String>,
    #[serde(default)]
    pub package_url: Option<String>,
    #[serde(default)]
    pub file_name: Option<String>,
    #[serde(default)]
    pub size: Option<i64>,
    #[serde(default)]
    pub sha256: Option<String>,
    #[serde(default)]
    pub signature: Option<String>,
    #[serde(default)]
    pub root_hash: Option<String>,
    #[serde(default)]
    pub compression: Option<String>,
    #[serde(default)]
    pub files: Option<Vec<IntegrityFile>>,
    #[serde(default)]
    pub deleted_paths: Option<Vec<String>>,
    #[serde(default)]
    pub invalid_paths: Option<Vec<String>>,
    #[serde(default)]
    pub version_integer: Option<i64>,
    #[serde(default)]
    pub version_semver: Option<String>,
    #[serde(default)]
    pub channel: Option<String>,
    #[serde(default)]
    pub compare_engine: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TelemetryReport {
    pub os: String,
    pub arch: String,
    pub channel: String,
    pub from_version: String,
    pub to_version: String,
    pub status: String,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub device_id: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub diff_mode: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub error_code: Option<String>,
    #[serde(skip_serializing_if = "skip_empty_str")]
    pub error_message: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct ProjectPublic {
    #[serde(default)]
    pub uuid: Option<String>,
    #[serde(default)]
    pub slug: Option<String>,
    #[serde(default)]
    pub compare_engine: Option<String>,
    #[serde(default)]
    pub default_locale: Option<String>,
    #[serde(default)]
    pub device_id_policy: Option<String>,
    #[serde(default)]
    pub force_https: Option<bool>,
    #[serde(default)]
    pub minimum_supported_version: Option<String>,
    #[serde(default)]
    pub require_client_token: Option<bool>,
    #[serde(default)]
    pub storage_visibility: Option<String>,
    #[serde(default)]
    pub created_at: Option<String>,
    #[serde(default)]
    pub updated_at: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct Channel {
    pub slug: String,
    #[serde(default)]
    pub name: Option<String>,
    #[serde(default)]
    pub stability_rank: Option<i64>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct ChannelList {
    #[serde(default)]
    pub channels: Vec<Channel>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct MatrixRow {
    pub os: String,
    pub arch: String,
    #[serde(default)]
    pub package_type: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct MatrixList {
    #[serde(default)]
    pub matrix: Vec<MatrixRow>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct Language {
    pub code: String,
    #[serde(default)]
    pub display_name: Option<String>,
    #[serde(default)]
    pub is_default: Option<bool>,
    #[serde(default)]
    pub sort_order: Option<i64>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct LanguageList {
    #[serde(default)]
    pub languages: Vec<Language>,
}

#[derive(Debug, Clone, Default)]
pub struct AnnouncementQuery {
    pub version: Option<String>,
    pub os: Option<String>,
    pub arch: Option<String>,
    pub locale: Option<String>,
    pub accept_language: Option<String>,
    pub if_none_match: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct Announcement {
    pub id: String,
    #[serde(default)]
    pub title: Option<String>,
    #[serde(default)]
    pub subtitle: Option<String>,
    #[serde(default)]
    pub markdown: Option<String>,
    #[serde(default)]
    pub locale: Option<String>,
    #[serde(default)]
    pub starts_at: Option<String>,
    #[serde(default)]
    pub ends_at: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct AnnouncementList {
    #[serde(default)]
    pub announcements: Vec<Announcement>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct Health {
    pub ready: bool,
    pub status: String,
}

#[derive(Debug, Clone)]
pub struct Download {
    pub status: u16,
    pub headers: Vec<(String, String)>,
    pub body: Vec<u8>,
}

#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ErrorEnvelope {
    pub error: Option<ErrorBody>,
}

#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ErrorBody {
    #[serde(default)]
    pub code: String,
    #[serde(default)]
    pub message: String,
    #[serde(default)]
    pub details: Option<Value>,
}

#[derive(Debug, Clone, Default)]
pub struct RequestOptions {
    pub if_none_match: Option<String>,
    pub range: Option<String>,
    pub accept_language: Option<String>,
}
