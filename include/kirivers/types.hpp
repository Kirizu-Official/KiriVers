#pragma once

#include "http.hpp"

#include <nlohmann/json.hpp>
#include <cstdint>
#include <optional>
#include <string>
#include <vector>

namespace kirivers {

struct HealthStatus {
  std::string status;
  bool ready = false;
};

struct ProjectPublic {
  std::string uuid;
  std::string slug;
  std::string compare_engine;
  std::string default_locale;
  std::string device_id_policy;
  std::string storage_visibility;
  bool force_https = false;
  bool require_client_token = false;
  std::optional<std::string> minimum_supported_version;
  std::string created_at;
  std::string updated_at;
};

struct DeviceReportInput {
  std::string device_id;
  std::optional<std::string> version;
  std::optional<std::string> os;
  std::optional<std::string> arch;
  std::optional<std::string> channel;
  nlohmann::json custom = nullptr;
};

struct DeviceReportOutput {
  std::string ip;
  std::string country_code;
  std::string region_code;
  nlohmann::json geo_i18n = nlohmann::json::object();
};

struct CheckInput {
  std::string current_version;
  std::string os;
  std::string arch;
  std::optional<std::string> channel;
  std::optional<std::string> hw_rev;
  std::optional<std::string> os_version;
  std::optional<std::string> device_id;
  // If unset, Client fills these from live adapters (D13).
  std::optional<std::vector<std::string>> capabilities;
  std::optional<std::vector<std::string>> accepted_delta_algos;
};

struct CheckOptions {
  std::optional<std::string> if_none_match;
};

struct UpdateCheckBody {
  bool has_update = false;
  bool is_mandatory = false;
  bool is_downgrade = false;
  std::string reason;
  std::string compare_engine;
  std::optional<std::int64_t> version_integer;
  std::optional<std::string> version_semver;
  std::string target_channel;
  std::optional<std::string> target_hw_rev;
  std::string package_type;
  std::string root_hash;
  std::string package_url;
  std::string file_name;
  std::int64_t size = 0;
  std::string sha256;
  bool delta_available = false;
  std::optional<std::string> delta_algo;
  std::optional<std::string> platform_notes;
  std::optional<std::string> publish_time;
  std::optional<std::string> signature;
  std::optional<std::string> artifact_signature;
};

enum class CheckStatus { Update, NoUpdate, NotModified };

struct CheckResult {
  CheckStatus status = CheckStatus::NoUpdate;
  std::optional<std::string> etag;
  std::optional<UpdateCheckBody> body;
};

struct ChangelogQuery {
  std::optional<std::string> from_version;
  std::optional<std::string> to_version;
  std::optional<std::string> changelog_scope;
  std::optional<std::string> changelog_layout;
  std::optional<bool> changelog_include_revoked;
  std::optional<bool> changelog_include_platform_notes;
  std::optional<std::string> changelog_locale;
  std::optional<std::string> locale;
  std::optional<std::string> if_none_match;
};

struct ChangelogVersion {
  std::string channel;
  std::string status;
  std::string changelog;
  bool had_artifact_for_request_platform = false;
  std::optional<std::int64_t> version_integer;
  std::optional<std::string> version_semver;
  std::optional<std::string> title;
  std::optional<std::string> platform_notes;
};

struct ChangelogBody {
  std::optional<std::string> changelog;
  std::vector<ChangelogVersion> changelog_versions;
  std::optional<std::string> etag;
  bool not_modified = false;
};

struct IntegrityQuery {
  std::string os;
  std::string arch;
  std::optional<std::string> channel;
  std::optional<std::string> hw_rev;
  std::optional<std::string> hash_algo;
  std::optional<bool> compact;
  std::optional<bool> include_file_urls;
  std::optional<std::string> if_none_match;
};

struct IntegrityFile {
  std::string path;
  std::int64_t size = 0;
  std::string install_policy;
  bool integrity_check = true;
  std::optional<std::string> sha256;
  std::optional<std::string> md5;
  std::optional<std::string> url;
};

struct IntegrityBody {
  std::optional<std::int64_t> version_integer;
  std::optional<std::string> version_semver;
  std::string channel;
  std::string package_type;
  std::string root_hash;
  std::string full_package_url;
  std::string file_name;
  std::int64_t size = 0;
  std::string sha256;
  std::vector<IntegrityFile> files;
  std::optional<std::string> signature;
  std::optional<std::string> etag;
  bool not_modified = false;
};

struct DiffInput {
  std::string source_version;
  std::string target_version;
  std::string os;
  std::string arch;
  std::optional<std::string> channel;
  std::optional<std::string> device_id;
  std::optional<std::string> hw_rev;
  std::optional<std::string> local_sha256;
  std::optional<bool> prefer_full;
  std::optional<std::vector<std::string>> capabilities;
  std::optional<std::vector<std::string>> accepted_delta_algos;
};

struct DiffFile {
  std::optional<std::string> path;
  std::optional<std::string> sha256;
  std::optional<std::string> md5;
  std::optional<std::int64_t> size;
  std::optional<std::string> url;
};

struct DiffBody {
  std::string diff_mode;
  std::string root_hash;
  std::optional<std::int64_t> version_integer;
  std::optional<std::string> version_semver;
  std::string channel;
  std::string compare_engine;
  std::optional<std::string> package_url;
  std::optional<std::string> file_name;
  std::optional<std::int64_t> size;
  std::optional<std::string> sha256;
  std::optional<std::string> delta_algo;
  std::optional<std::string> signature;
  std::vector<DiffFile> files;
  std::vector<std::string> deleted_paths;
  std::vector<std::string> invalid_paths;
};

struct PackInput {
  std::string source_version;
  std::string target_version;
  std::string os;
  std::string arch;
  std::optional<std::string> channel;
  std::optional<std::string> device_id;
  std::optional<std::string> hw_rev;
  std::vector<std::string> needed_paths;
};

struct PackFile {
  std::optional<std::string> path;
  std::optional<std::string> sha256;
  std::optional<std::int64_t> size;
  std::optional<std::string> install_policy;
  std::optional<bool> integrity_check;
};

struct PackBody {
  std::string status;
  std::optional<std::string> diff_mode;
  std::optional<std::string> compression;
  std::optional<std::string> package_url;
  std::optional<std::string> file_name;
  std::optional<std::int64_t> size;
  std::optional<std::string> sha256;
  std::optional<std::string> root_hash;
  std::optional<std::string> channel;
  std::optional<std::string> compare_engine;
  std::optional<std::int64_t> version_integer;
  std::optional<std::string> version_semver;
  std::optional<std::string> signature;
  std::vector<PackFile> files;
  std::vector<std::string> deleted_paths;
  std::vector<std::string> invalid_paths;
  int http_status = 200;
};

struct DownloadOptions {
  std::optional<std::string> range;  // e.g. "bytes=0-1023"
  std::optional<std::string> hw_rev;
};

struct TelemetryInput {
  std::string os;
  std::string arch;
  std::string channel;
  std::string from_version;
  std::string to_version;
  std::string status;  // downloading|applying|installed|failed|rolled_back
  std::optional<std::string> device_id;
  std::optional<std::string> diff_mode;
  std::optional<std::string> error_code;
  std::optional<std::string> error_message;
};

struct ChannelRow {
  std::string slug;
  std::string name;
  std::optional<int> stability_rank;
};

struct MatrixRow {
  std::string os;
  std::string arch;
  std::string package_type;
};

struct LanguageRow {
  std::string code;
  std::string display_name;
  bool is_default = false;
};

struct AnnouncementQuery {
  std::optional<std::string> version;
  std::optional<std::string> os;
  std::optional<std::string> arch;
  std::optional<std::string> locale;
  std::optional<std::string> accept_language;
  std::optional<std::string> if_none_match;
};

struct Announcement {
  std::string id;
  std::string title;
  std::optional<std::string> subtitle;
  std::optional<std::string> locale;
  std::optional<std::string> markdown;
  std::optional<std::string> starts_at;
  std::optional<std::string> ends_at;
};

struct AnnouncementList {
  std::vector<Announcement> announcements;
  std::optional<std::string> etag;
  bool not_modified = false;
};

struct DeclaredCapabilities {
  std::vector<std::string> capabilities;
  std::vector<std::string> accepted_delta_algos;
};

HealthStatus parse_health(const nlohmann::json& j);
ProjectPublic parse_project_public(const nlohmann::json& j);
DeviceReportOutput parse_device_report(const nlohmann::json& j);
UpdateCheckBody parse_update_check(const nlohmann::json& j);
ChangelogBody parse_changelog(const nlohmann::json& j);
IntegrityBody parse_integrity(const nlohmann::json& j);
DiffBody parse_diff(const nlohmann::json& j);
PackBody parse_pack(const nlohmann::json& j);
std::vector<ChannelRow> parse_channels(const nlohmann::json& j);
std::vector<MatrixRow> parse_matrix(const nlohmann::json& j);
std::vector<LanguageRow> parse_languages(const nlohmann::json& j);
AnnouncementList parse_announcements(const nlohmann::json& j);

}  // namespace kirivers
