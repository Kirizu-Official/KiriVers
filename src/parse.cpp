#include <kirivers/types.hpp>

namespace kirivers {

namespace {

std::optional<std::string> opt_str(const nlohmann::json& j, const char* k) {
  if (!j.contains(k) || j[k].is_null()) return std::nullopt;
  if (j[k].is_string()) return j[k].get<std::string>();
  return j[k].dump();
}

std::optional<std::int64_t> opt_int(const nlohmann::json& j, const char* k) {
  if (!j.contains(k) || j[k].is_null()) return std::nullopt;
  if (j[k].is_number_integer()) return j[k].get<std::int64_t>();
  if (j[k].is_string()) {
    try {
      return std::stoll(j[k].get<std::string>());
    } catch (...) {
      return std::nullopt;
    }
  }
  return std::nullopt;
}

std::string str(const nlohmann::json& j, const char* k) {
  auto v = opt_str(j, k);
  return v ? *v : std::string{};
}

bool boolean(const nlohmann::json& j, const char* k, bool def = false) {
  if (!j.contains(k) || j[k].is_null()) return def;
  if (j[k].is_boolean()) return j[k].get<bool>();
  return def;
}

}  // namespace

HealthStatus parse_health(const nlohmann::json& j) {
  HealthStatus h;
  h.status = str(j, "status");
  h.ready = boolean(j, "ready");
  return h;
}

ProjectPublic parse_project_public(const nlohmann::json& j) {
  ProjectPublic p;
  p.uuid = str(j, "uuid");
  p.slug = str(j, "slug");
  p.compare_engine = str(j, "compare_engine");
  p.default_locale = str(j, "default_locale");
  p.device_id_policy = str(j, "device_id_policy");
  p.storage_visibility = str(j, "storage_visibility");
  p.force_https = boolean(j, "force_https");
  p.require_client_token = boolean(j, "require_client_token");
  p.minimum_supported_version = opt_str(j, "minimum_supported_version");
  p.created_at = str(j, "created_at");
  p.updated_at = str(j, "updated_at");
  return p;
}

DeviceReportOutput parse_device_report(const nlohmann::json& j) {
  DeviceReportOutput o;
  o.ip = str(j, "ip");
  o.country_code = str(j, "country_code");
  o.region_code = str(j, "region_code");
  if (j.contains("geo_i18n") && j["geo_i18n"].is_object()) {
    o.geo_i18n = j["geo_i18n"];
  }
  return o;
}

UpdateCheckBody parse_update_check(const nlohmann::json& j) {
  UpdateCheckBody b;
  b.has_update = boolean(j, "has_update");
  b.is_mandatory = boolean(j, "is_mandatory");
  b.is_downgrade = boolean(j, "is_downgrade");
  b.reason = str(j, "reason");
  b.compare_engine = str(j, "compare_engine");
  b.version_integer = opt_int(j, "version_integer");
  b.version_semver = opt_str(j, "version_semver");
  b.target_channel = str(j, "target_channel");
  b.target_hw_rev = opt_str(j, "target_hw_rev");
  b.package_type = str(j, "package_type");
  b.root_hash = str(j, "root_hash");
  b.package_url = str(j, "package_url");
  b.file_name = str(j, "file_name");
  b.size = opt_int(j, "size").value_or(0);
  b.sha256 = str(j, "sha256");
  b.delta_available = boolean(j, "delta_available");
  b.delta_algo = opt_str(j, "delta_algo");
  b.platform_notes = opt_str(j, "platform_notes");
  b.publish_time = opt_str(j, "publish_time");
  b.signature = opt_str(j, "signature");
  b.artifact_signature = opt_str(j, "artifact_signature");
  return b;
}

ChangelogBody parse_changelog(const nlohmann::json& j) {
  ChangelogBody b;
  b.changelog = opt_str(j, "changelog");
  if (j.contains("changelog_versions") && j["changelog_versions"].is_array()) {
    for (const auto& v : j["changelog_versions"]) {
      ChangelogVersion row;
      row.channel = str(v, "channel");
      row.status = str(v, "status");
      row.changelog = str(v, "changelog");
      row.had_artifact_for_request_platform =
          boolean(v, "had_artifact_for_request_platform");
      row.version_integer = opt_int(v, "version_integer");
      row.version_semver = opt_str(v, "version_semver");
      row.title = opt_str(v, "title");
      row.platform_notes = opt_str(v, "platform_notes");
      b.changelog_versions.push_back(std::move(row));
    }
  }
  return b;
}

IntegrityBody parse_integrity(const nlohmann::json& j) {
  IntegrityBody b;
  b.version_integer = opt_int(j, "version_integer");
  b.version_semver = opt_str(j, "version_semver");
  b.channel = str(j, "channel");
  b.package_type = str(j, "package_type");
  b.root_hash = str(j, "root_hash");
  b.full_package_url = str(j, "full_package_url");
  b.file_name = str(j, "file_name");
  b.size = opt_int(j, "size").value_or(0);
  b.sha256 = str(j, "sha256");
  b.signature = opt_str(j, "signature");
  if (j.contains("files") && j["files"].is_array()) {
    for (const auto& f : j["files"]) {
      IntegrityFile row;
      row.path = str(f, "path");
      row.size = opt_int(f, "size").value_or(0);
      row.install_policy = str(f, "install_policy");
      row.integrity_check = boolean(f, "integrity_check", true);
      row.sha256 = opt_str(f, "sha256");
      row.md5 = opt_str(f, "md5");
      row.url = opt_str(f, "url");
      b.files.push_back(std::move(row));
    }
  }
  return b;
}

DiffBody parse_diff(const nlohmann::json& j) {
  DiffBody b;
  b.diff_mode = str(j, "diff_mode");
  b.root_hash = str(j, "root_hash");
  b.version_integer = opt_int(j, "version_integer");
  b.version_semver = opt_str(j, "version_semver");
  b.channel = str(j, "channel");
  b.compare_engine = str(j, "compare_engine");
  b.package_url = opt_str(j, "package_url");
  b.file_name = opt_str(j, "file_name");
  b.size = opt_int(j, "size");
  b.sha256 = opt_str(j, "sha256");
  b.delta_algo = opt_str(j, "delta_algo");
  b.signature = opt_str(j, "signature");
  if (j.contains("files") && j["files"].is_array()) {
    for (const auto& f : j["files"]) {
      DiffFile row;
      row.path = opt_str(f, "path");
      row.sha256 = opt_str(f, "sha256");
      row.md5 = opt_str(f, "md5");
      row.size = opt_int(f, "size");
      row.url = opt_str(f, "url");
      b.files.push_back(std::move(row));
    }
  }
  if (j.contains("deleted_paths") && j["deleted_paths"].is_array()) {
    for (const auto& p : j["deleted_paths"]) {
      if (p.is_string()) b.deleted_paths.push_back(p.get<std::string>());
    }
  }
  if (j.contains("invalid_paths") && j["invalid_paths"].is_array()) {
    for (const auto& p : j["invalid_paths"]) {
      if (p.is_string()) b.invalid_paths.push_back(p.get<std::string>());
    }
  }
  return b;
}

PackBody parse_pack(const nlohmann::json& j) {
  PackBody b;
  b.status = str(j, "status");
  b.diff_mode = opt_str(j, "diff_mode");
  b.compression = opt_str(j, "compression");
  b.package_url = opt_str(j, "package_url");
  b.file_name = opt_str(j, "file_name");
  b.size = opt_int(j, "size");
  b.sha256 = opt_str(j, "sha256");
  b.root_hash = opt_str(j, "root_hash");
  b.channel = opt_str(j, "channel");
  b.compare_engine = opt_str(j, "compare_engine");
  b.version_integer = opt_int(j, "version_integer");
  b.version_semver = opt_str(j, "version_semver");
  b.signature = opt_str(j, "signature");
  if (j.contains("files") && j["files"].is_array()) {
    for (const auto& f : j["files"]) {
      PackFile row;
      row.path = opt_str(f, "path");
      row.sha256 = opt_str(f, "sha256");
      row.size = opt_int(f, "size");
      row.install_policy = opt_str(f, "install_policy");
      if (f.contains("integrity_check") && f["integrity_check"].is_boolean()) {
        row.integrity_check = f["integrity_check"].get<bool>();
      }
      b.files.push_back(std::move(row));
    }
  }
  if (j.contains("deleted_paths") && j["deleted_paths"].is_array()) {
    for (const auto& p : j["deleted_paths"]) {
      if (p.is_string()) b.deleted_paths.push_back(p.get<std::string>());
    }
  }
  if (j.contains("invalid_paths") && j["invalid_paths"].is_array()) {
    for (const auto& p : j["invalid_paths"]) {
      if (p.is_string()) b.invalid_paths.push_back(p.get<std::string>());
    }
  }
  return b;
}

std::vector<ChannelRow> parse_channels(const nlohmann::json& j) {
  std::vector<ChannelRow> out;
  const nlohmann::json* arr = nullptr;
  if (j.contains("channels") && j["channels"].is_array()) arr = &j["channels"];
  else if (j.is_array()) arr = &j;
  if (!arr) return out;
  for (const auto& c : *arr) {
    ChannelRow row;
    row.slug = str(c, "slug");
    row.name = str(c, "name");
    if (auto n = opt_int(c, "stability_rank")) row.stability_rank = static_cast<int>(*n);
    out.push_back(std::move(row));
  }
  return out;
}

std::vector<MatrixRow> parse_matrix(const nlohmann::json& j) {
  std::vector<MatrixRow> out;
  const nlohmann::json* arr = nullptr;
  if (j.contains("matrix") && j["matrix"].is_array()) arr = &j["matrix"];
  else if (j.is_array()) arr = &j;
  if (!arr) return out;
  for (const auto& c : *arr) {
    MatrixRow row;
    row.os = str(c, "os");
    row.arch = str(c, "arch");
    row.package_type = str(c, "package_type");
    out.push_back(std::move(row));
  }
  return out;
}

std::vector<LanguageRow> parse_languages(const nlohmann::json& j) {
  std::vector<LanguageRow> out;
  const nlohmann::json* arr = nullptr;
  if (j.contains("languages") && j["languages"].is_array()) arr = &j["languages"];
  else if (j.is_array()) arr = &j;
  if (!arr) return out;
  for (const auto& c : *arr) {
    LanguageRow row;
    row.code = str(c, "code");
    row.display_name = str(c, "display_name");
    row.is_default = boolean(c, "is_default");
    out.push_back(std::move(row));
  }
  return out;
}

AnnouncementList parse_announcements(const nlohmann::json& j) {
  AnnouncementList out;
  const nlohmann::json* arr = nullptr;
  if (j.contains("announcements") && j["announcements"].is_array()) {
    arr = &j["announcements"];
  } else if (j.is_array()) {
    arr = &j;
  }
  if (!arr) return out;
  for (const auto& a : *arr) {
    Announcement row;
    row.id = str(a, "id");
    row.title = str(a, "title");
    row.subtitle = opt_str(a, "subtitle");
    row.locale = opt_str(a, "locale");
    row.markdown = opt_str(a, "markdown");
    row.starts_at = opt_str(a, "starts_at");
    row.ends_at = opt_str(a, "ends_at");
    out.announcements.push_back(std::move(row));
  }
  return out;
}

}  // namespace kirivers
