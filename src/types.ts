/** OpenAPI snapshot version plus short digest of `openapi.client.json`. */
export const OPENAPI_REVISION = "1.0.0 B443DEA6";

export const CAPABILITY_FULL_PACKAGE = "full_package";
export const CAPABILITY_PATCH_PACKAGE = "patch_package";
export const CAPABILITY_FILE_LIST = "file_list";
export const CAPABILITY_BINARY_DELTA = "binary_delta";

export type CompareEngine = "semver" | "integer";
export type PackageType = "single_file" | "multi_file";
export type DiffMode = "full_package" | "patch_package" | "binary_delta" | "file_list";
export type PackStatus = "pending" | "ready" | "full_package";
export type TelemetryStatus = "downloading" | "applying" | "installed" | "failed" | "rolled_back";
export type DeviceIdPolicy = "hashed" | "raw" | "none";
export type HashAlgo = "sha256" | "md5" | "both";
export type SignAlgo = "ed25519" | "rsa-sha256";
export type DeltaAlgo = "hdiffpatch" | "bsdiff" | "xdelta3";

export interface UpdateCheckRequest {
  current_version: string;
  os: string;
  arch: string;
  channel?: string;
  hw_rev?: string;
  os_version?: string;
  device_id?: string;
  capabilities?: string[];
  accepted_delta_algos?: string[];
}

export interface UpdateCheck200 {
  has_update: boolean;
  is_mandatory: boolean;
  is_downgrade: boolean;
  reason: string;
  compare_engine: CompareEngine;
  version_integer: number | null;
  version_semver: string | null;
  target_channel: string;
  target_hw_rev: string | null;
  package_type: PackageType;
  root_hash: string;
  package_url: string;
  file_name: string;
  size: number;
  sha256: string;
  delta_available: boolean;
  delta_algo?: string;
  platform_notes?: string;
  publish_time?: string | null;
  signature?: string;
  artifact_signature?: string;
}

export type CheckResult =
  | { kind: "update"; status: 200; body: UpdateCheck200; etag?: string; headers: Record<string, string> }
  | { kind: "no_update"; status: 204; etag?: string; headers: Record<string, string> }
  | { kind: "not_modified"; status: 304; etag?: string; headers: Record<string, string> };

export interface DeviceReportInput {
  device_id: string;
  version?: string;
  os?: string;
  arch?: string;
  channel?: string;
  custom?: Record<string, unknown>;
}

export interface DeviceReportOutput {
  ip: string;
  country_code: string;
  region_code: string;
  geo_i18n: Record<string, unknown>;
}

export interface ChangelogQuery {
  from_version?: string;
  to_version?: string;
  changelog_scope?: "range_all" | "range_platform" | "target_only";
  changelog_layout?: "aggregated" | "structured" | "both";
  changelog_include_revoked?: boolean;
  changelog_include_platform_notes?: boolean;
  changelog_locale?: string;
  locale?: string;
  ifNoneMatch?: string;
}

export interface ChangelogVersion {
  changelog: string;
  channel: string;
  had_artifact_for_request_platform: boolean;
  platform_notes?: string;
  status: string;
  title?: string;
  version_integer: number | null;
  version_semver: string | null;
}

export interface ChangelogBody {
  changelog?: string;
  changelog_versions?: ChangelogVersion[];
}

export interface IntegrityQuery {
  os: string;
  arch: string;
  hash_algo?: HashAlgo;
  compact?: boolean;
  include_file_urls?: boolean;
  hw_rev?: string;
  channel?: string;
  ifNoneMatch?: string;
}

export interface ManifestFile {
  path: string;
  size: number;
  install_policy: "OVERWRITE" | "KEEP_IF_EXISTS" | string;
  integrity_check: boolean;
  sha256?: string;
  md5?: string;
  url?: string;
}

export interface Integrity200 {
  version_integer: number | null;
  version_semver: string | null;
  channel: string;
  package_type: PackageType;
  root_hash: string;
  full_package_url: string;
  file_name: string;
  size: number;
  sha256: string;
  files: ManifestFile[];
  signature?: string;
  volumes?: Array<{ sha256: string; size: number; url?: string }> | null;
}

export interface DiffRequest {
  source_version: string;
  target_version: string;
  os: string;
  arch: string;
  channel?: string;
  device_id?: string;
  hw_rev?: string;
  local_sha256?: string;
  capabilities?: string[];
  accepted_delta_algos?: string[];
  prefer_full?: boolean;
}

export interface Diff200 {
  diff_mode: DiffMode;
  root_hash: string;
  version_integer: number | null;
  version_semver: string | null;
  channel: string;
  compare_engine: CompareEngine;
  package_url?: string;
  sha256?: string;
  size?: number;
  file_name?: string;
  delta_algo?: string;
  signature?: string;
  files?: Array<{ path?: string; sha256?: string; md5?: string; size?: number; url?: string }>;
  deleted_paths?: string[];
  invalid_paths?: string[];
}

export interface PackRequest {
  source_version: string;
  target_version: string;
  os: string;
  arch: string;
  channel?: string;
  device_id?: string;
  hw_rev?: string;
  needed_paths?: string[];
}

export interface Pack200 {
  status: PackStatus;
  channel?: string;
  compare_engine?: CompareEngine;
  compression?: string;
  deleted_paths?: string[];
  diff_mode?: "full_package" | "patch_package";
  file_name?: string;
  files?: Array<{
    path?: string;
    sha256?: string;
    size?: number;
    install_policy?: string;
    integrity_check?: boolean;
  }>;
  invalid_paths?: string[];
  package_url?: string;
  root_hash?: string;
  sha256?: string;
  signature?: string;
  size?: number;
  version_integer?: number | null;
  version_semver?: string | null;
}

export interface TelemetryReport {
  os: string;
  arch: string;
  channel: string;
  from_version: string;
  to_version: string;
  status: TelemetryStatus;
  device_id?: string;
  diff_mode?: DiffMode;
  error_code?: string;
  error_message?: string;
}

export interface ProjectPublic {
  uuid?: string;
  slug?: string;
  compare_engine?: string;
  created_at?: string;
  updated_at?: string;
  default_locale?: string;
  device_id_policy?: DeviceIdPolicy;
  force_https?: boolean;
  minimum_supported_version?: string | null;
  require_client_token?: boolean;
  storage_visibility?: string;
}

export interface ClientChannel {
  name?: string;
  slug?: string;
  stability_rank?: number;
}

export interface ClientMatrixRow {
  os?: string;
  arch?: string;
  package_type?: PackageType;
}

export interface ClientLanguage {
  code?: string;
  display_name?: string;
  is_default?: boolean;
  sort_order?: number;
}

export interface ClientAnnouncement {
  id?: string;
  locale?: string;
  markdown?: string;
  title?: string;
  subtitle?: string;
  starts_at?: string | null;
  ends_at?: string | null;
}

export interface AnnouncementQuery {
  version?: string;
  os?: string;
  arch?: string;
  locale?: string;
  acceptLanguage?: string;
  ifNoneMatch?: string;
}

export interface HealthStatus {
  status: string;
  ready: boolean;
}

export interface DownloadOptions {
  range?: string;
  exp?: string;
  sig?: string;
  hw_rev?: string;
}

export interface BinaryResult {
  status: number;
  body: Uint8Array;
  headers: Record<string, string>;
}

export interface JsonResult<T> {
  status: number;
  body: T;
  etag?: string;
  headers: Record<string, string>;
}

export interface VerifyKey {
  algo: SignAlgo;
  pem: string;
}

export interface UnpackFile {
  path: string;
  sha256: string;
}
