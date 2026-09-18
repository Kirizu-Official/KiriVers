#include <kirivers/client.hpp>
#include <kirivers/hosted.hpp>

#include "url.hpp"

#include <nlohmann/json.hpp>
#include <algorithm>
#include <chrono>
#include <optional>
#include <set>
#include <thread>
#include <vector>

namespace kirivers {

namespace {

void put_opt(nlohmann::json& j, const char* k, const std::optional<std::string>& v) {
  if (v && !v->empty()) j[k] = *v;
}

std::vector<std::string> unique_needed(const std::vector<std::string>& paths,
                                       FileStore* store) {
  std::vector<std::string> needed;
  std::set<std::string> seen;
  for (const auto& p : paths) {
    std::optional<std::string> n;
    try {
      n = store ? std::optional<std::string>(store->normalize_path(p))
                : normalize_rel_path(p);
    } catch (...) {
      continue;
    }
    if (!n) continue;
    if (seen.insert(*n).second) needed.push_back(*n);
  }
  return needed;
}

}  // namespace

#if defined(KIRIVERS_HOSTED)
void apply_hosted_defaults(Config& cfg);
#endif

Client::Client(Config cfg) : cfg_(std::move(cfg)) {
#if defined(KIRIVERS_HOSTED)
  if (cfg_.hosted_defaults) apply_hosted_defaults(cfg_);
#endif
  if (!cfg_.transport) {
    throw ConfigError("Transport is required (inject one, or use the hosted preset)");
  }
  cfg_.base_url = detail::trim_slash(cfg_.base_url);
}

DeclaredCapabilities Client::declared_capabilities() const {
  DeclaredCapabilities d;
  d.capabilities.push_back("full_package");
  if (cfg_.archive_unpacker) d.capabilities.push_back("patch_package");
  if (cfg_.file_store && cfg_.file_store->can_write_individual_files()) {
    d.capabilities.push_back("file_list");
  }
  if (cfg_.patcher) {
    d.accepted_delta_algos = cfg_.patcher->supported_algos();
    if (!d.accepted_delta_algos.empty()) {
      d.capabilities.push_back("binary_delta");
    }
  }
  return d;
}

void Client::apply_auth(HeaderMap& h) const {
  if (!cfg_.project_token.empty()) {
    h.emplace("Authorization", "Bearer " + cfg_.project_token);
    h.emplace("X-Project-Token", cfg_.project_token);
  }
  if (!cfg_.channel_token.empty()) {
    h.emplace("X-Channel-Token", cfg_.channel_token);
  }
  h.emplace("User-Agent", "kirivers-client-cpp/" KIRIVERS_CLIENT_VERSION);
  h.emplace("Accept", "application/json");
}

std::string Client::project_url(const std::string& suffix) const {
  std::string path = "/api/v1/projects/" + detail::url_encode(cfg_.project_ref);
  if (!suffix.empty()) {
    if (suffix.front() != '/') path.push_back('/');
    path += suffix;
  }
  return detail::join_url(cfg_.base_url, path);
}

std::string Client::resolve_download_url(const std::string& url_or_sha256) const {
  if (detail::is_absolute_url(url_or_sha256)) return url_or_sha256;
  if (!url_or_sha256.empty() && url_or_sha256.front() == '/') {
    return detail::join_url(cfg_.base_url, url_or_sha256);
  }
  return project_url("packages/" + detail::url_encode(url_or_sha256));
}

HttpResponse Client::execute(const std::string& method, const std::string& url,
                             const HeaderMap& extra, const Bytes& body,
                             const std::string& content_type) {
  HttpRequest req;
  req.method = method;
  req.url = url;
  apply_auth(req.headers);
  for (const auto& kv : extra) req.headers[kv.first] = kv.second;
  if (!content_type.empty()) req.headers["Content-Type"] = content_type;
  req.body = body;
  return cfg_.transport->execute(req);
}

HttpResponse Client::execute_json(const std::string& method, const std::string& url,
                                  const nlohmann::json& body) {
  auto dumped = body.dump();
  return execute(method, url, {}, string_to_bytes(dumped), "application/json");
}

void Client::throw_if_error(const HttpResponse& resp) const {
  if (resp.status < 400) return;
  throw ApiError::from_body(resp.status, bytes_to_string(resp.body));
}

nlohmann::json Client::parse_success_json(const HttpResponse& resp) const {
  throw_if_error(resp);
  if (resp.body.empty()) return nlohmann::json::object();
  try {
    return nlohmann::json::parse(bytes_to_string(resp.body));
  } catch (const nlohmann::json::exception& ex) {
    throw ApiError(resp.status, "INVALID_RESPONSE", ex.what());
  }
}

HealthStatus Client::health() {
  auto resp = execute("GET", detail::join_url(cfg_.base_url, "/api/v1/health"));
  return parse_health(parse_success_json(resp));
}

ProjectPublic Client::project() {
  auto resp = execute("GET", project_url(""));
  return parse_project_public(parse_success_json(resp));
}

DeviceReportOutput Client::report_device(const DeviceReportInput& in) {
  nlohmann::json body;
  body["device_id"] = in.device_id;
  put_opt(body, "version", in.version);
  put_opt(body, "os", in.os);
  put_opt(body, "arch", in.arch);
  put_opt(body, "channel", in.channel);
  if (!in.custom.is_null()) body["custom"] = in.custom;
  auto resp = execute_json("POST", project_url("clients/report"), body);
  return parse_device_report(parse_success_json(resp));
}

CheckResult Client::check(const CheckInput& in, const CheckOptions& opt) {
  nlohmann::json body;
  body["current_version"] = in.current_version;
  body["os"] = in.os;
  body["arch"] = in.arch;
  put_opt(body, "channel", in.channel);
  put_opt(body, "hw_rev", in.hw_rev);
  put_opt(body, "os_version", in.os_version);
  put_opt(body, "device_id", in.device_id);

  auto derived = declared_capabilities();
  std::vector<std::string> caps =
      in.capabilities ? *in.capabilities : derived.capabilities;
  std::vector<std::string> algos = in.accepted_delta_algos
                                       ? *in.accepted_delta_algos
                                       : derived.accepted_delta_algos;
  body["capabilities"] = caps;
  if (!algos.empty()) body["accepted_delta_algos"] = algos;

  HeaderMap extra;
  if (opt.if_none_match) extra["If-None-Match"] = *opt.if_none_match;
  auto resp = execute("POST", project_url("update/check"), extra,
                      string_to_bytes(body.dump()), "application/json");
  CheckResult out;
  auto et = header_value(resp.headers, "ETag");
  if (!et.empty()) out.etag = et;
  if (resp.status == 304) {
    out.status = CheckStatus::NotModified;
    return out;
  }
  if (resp.status == 204) {
    out.status = CheckStatus::NoUpdate;
    return out;
  }
  auto j = parse_success_json(resp);
  out.body = parse_update_check(j);
  out.status = CheckStatus::Update;
  return out;
}

ChangelogBody Client::changelog(const std::string& channel, const std::string& os,
                                const std::string& arch, const ChangelogQuery& q) {
  std::string url = project_url("changelog/" + detail::url_encode(channel) + "/" +
                                detail::url_encode(os) + "/" + detail::url_encode(arch));
  auto add = [&](const char* k, const std::optional<std::string>& v) {
    if (v && !v->empty()) url = detail::add_query(url, k, *v);
  };
  add("from_version", q.from_version);
  add("to_version", q.to_version);
  add("changelog_scope", q.changelog_scope);
  add("changelog_layout", q.changelog_layout);
  add("changelog_locale", q.changelog_locale);
  add("locale", q.locale);
  if (q.changelog_include_revoked) {
    url = detail::add_query(url, "changelog_include_revoked",
                            *q.changelog_include_revoked ? "true" : "false");
  }
  if (q.changelog_include_platform_notes) {
    url = detail::add_query(url, "changelog_include_platform_notes",
                            *q.changelog_include_platform_notes ? "true" : "false");
  }
  HeaderMap extra;
  if (q.if_none_match) extra["If-None-Match"] = *q.if_none_match;
  auto resp = execute("GET", url, extra);
  if (resp.status == 304) {
    ChangelogBody b;
    b.not_modified = true;
    auto et = header_value(resp.headers, "ETag");
    if (!et.empty()) b.etag = et;
    return b;
  }
  auto parsed = parse_changelog(parse_success_json(resp));
  auto et = header_value(resp.headers, "ETag");
  if (!et.empty()) parsed.etag = et;
  return parsed;
}

IntegrityBody Client::integrity(const std::string& version, const IntegrityQuery& q) {
  std::string url = project_url("versions/" + detail::url_encode(version) + "/integrity");
  url = detail::add_query(url, "os", q.os);
  url = detail::add_query(url, "arch", q.arch);
  if (q.channel) url = detail::add_query(url, "channel", *q.channel);
  if (q.hw_rev) url = detail::add_query(url, "hw_rev", *q.hw_rev);
  if (q.hash_algo) url = detail::add_query(url, "hash_algo", *q.hash_algo);
  if (q.compact) {
    url = detail::add_query(url, "compact", *q.compact ? "true" : "false");
  }
  if (q.include_file_urls) {
    url = detail::add_query(url, "include_file_urls",
                            *q.include_file_urls ? "true" : "false");
  }
  HeaderMap extra;
  if (q.if_none_match) extra["If-None-Match"] = *q.if_none_match;
  auto resp = execute("GET", url, extra);
  if (resp.status == 304) {
    IntegrityBody b;
    b.not_modified = true;
    auto et = header_value(resp.headers, "ETag");
    if (!et.empty()) b.etag = et;
    return b;
  }
  auto parsed = parse_integrity(parse_success_json(resp));
  auto et = header_value(resp.headers, "ETag");
  if (!et.empty()) parsed.etag = et;
  return parsed;
}

DiffBody Client::diff(const DiffInput& in) {
  nlohmann::json body;
  body["source_version"] = in.source_version;
  body["target_version"] = in.target_version;
  body["os"] = in.os;
  body["arch"] = in.arch;
  put_opt(body, "channel", in.channel);
  put_opt(body, "device_id", in.device_id);
  put_opt(body, "hw_rev", in.hw_rev);
  put_opt(body, "local_sha256", in.local_sha256);
  if (in.prefer_full) body["prefer_full"] = *in.prefer_full;
  auto derived = declared_capabilities();
  body["capabilities"] = in.capabilities ? *in.capabilities : derived.capabilities;
  auto algos = in.accepted_delta_algos ? *in.accepted_delta_algos
                                       : derived.accepted_delta_algos;
  if (!algos.empty()) body["accepted_delta_algos"] = algos;
  auto resp = execute_json("POST", project_url("update/diff"), body);
  return parse_diff(parse_success_json(resp));
}

PackBody Client::pack(const PackInput& in) {
  nlohmann::json body;
  body["source_version"] = in.source_version;
  body["target_version"] = in.target_version;
  body["os"] = in.os;
  body["arch"] = in.arch;
  put_opt(body, "channel", in.channel);
  put_opt(body, "device_id", in.device_id);
  put_opt(body, "hw_rev", in.hw_rev);
  body["needed_paths"] = unique_needed(in.needed_paths, cfg_.file_store.get());
  auto resp = execute_json("POST", project_url("update/pack"), body);
  auto parsed = parse_pack(parse_success_json(resp));
  parsed.http_status = resp.status;
  return parsed;
}

PackBody Client::pack_wait(const PackInput& in, const PackWaitOptions& opt) {
  nlohmann::json body;
  body["source_version"] = in.source_version;
  body["target_version"] = in.target_version;
  body["os"] = in.os;
  body["arch"] = in.arch;
  put_opt(body, "channel", in.channel);
  put_opt(body, "device_id", in.device_id);
  put_opt(body, "hw_rev", in.hw_rev);
  body["needed_paths"] = unique_needed(in.needed_paths, cfg_.file_store.get());
  const std::string dumped = body.dump();
  const auto start = std::chrono::steady_clock::now();
  auto delay = opt.initial;
  while (true) {
    auto resp = execute("POST", project_url("update/pack"), {},
                        string_to_bytes(dumped), "application/json");
    auto parsed = parse_pack(parse_success_json(resp));
    parsed.http_status = resp.status;
    const bool pending =
        resp.status == 202 || parsed.status == "pending";
    if (!pending) return parsed;
    if (std::chrono::steady_clock::now() - start > opt.deadline) {
      throw ApiError(resp.status, "PACK_TIMEOUT", "pack poll deadline exceeded");
    }
    std::this_thread::sleep_for(delay);
    if (delay < opt.cap) delay = std::min(delay * 2, opt.cap);
  }
}

Bytes Client::download(const std::string& url_or_sha256, const DownloadOptions& opt) {
  std::string url = resolve_download_url(url_or_sha256);
  if (opt.hw_rev) url = detail::add_query(url, "hw_rev", *opt.hw_rev);
  HeaderMap extra;
  extra["Accept"] = "application/octet-stream";
  if (opt.range) extra["Range"] = *opt.range;
  auto resp = execute("GET", url, extra);
  if (resp.status != 200 && resp.status != 206) throw_if_error(resp);
  if (resp.status != 200 && resp.status != 206) {
    throw ApiError(resp.status, "HTTP_ERROR", "unexpected download status");
  }
  return resp.body;
}

HttpResponse Client::head_package(const std::string& url_or_sha256,
                                  const DownloadOptions& opt) {
  std::string url = resolve_download_url(url_or_sha256);
  if (opt.hw_rev) url = detail::add_query(url, "hw_rev", *opt.hw_rev);
  HeaderMap extra;
  extra["Accept"] = "application/octet-stream";
  if (opt.range) extra["Range"] = *opt.range;
  auto resp = execute("HEAD", url, extra);
  throw_if_error(resp);
  return resp;
}

std::vector<ChannelRow> Client::channels() {
  auto resp = execute("GET", project_url("channels"));
  return parse_channels(parse_success_json(resp));
}

std::vector<MatrixRow> Client::matrix() {
  auto resp = execute("GET", project_url("matrix"));
  return parse_matrix(parse_success_json(resp));
}

std::vector<LanguageRow> Client::languages() {
  auto resp = execute("GET", project_url("languages"));
  return parse_languages(parse_success_json(resp));
}

AnnouncementList Client::announcements(const AnnouncementQuery& q) {
  std::string url = project_url("announcements");
  if (q.version) url = detail::add_query(url, "version", *q.version);
  if (q.os) url = detail::add_query(url, "os", *q.os);
  if (q.arch) url = detail::add_query(url, "arch", *q.arch);
  if (q.locale) url = detail::add_query(url, "locale", *q.locale);
  HeaderMap extra;
  if (q.accept_language) extra["Accept-Language"] = *q.accept_language;
  if (q.if_none_match) extra["If-None-Match"] = *q.if_none_match;
  auto resp = execute("GET", url, extra);
  if (resp.status == 304) {
    AnnouncementList l;
    l.not_modified = true;
    auto et = header_value(resp.headers, "ETag");
    if (!et.empty()) l.etag = et;
    return l;
  }
  auto parsed = parse_announcements(parse_success_json(resp));
  auto et = header_value(resp.headers, "ETag");
  if (!et.empty()) parsed.etag = et;
  return parsed;
}

void Client::report_telemetry(const TelemetryInput& in) {
  nlohmann::json body;
  body["os"] = in.os;
  body["arch"] = in.arch;
  body["channel"] = in.channel;
  body["from_version"] = in.from_version;
  body["to_version"] = in.to_version;
  body["status"] = in.status;
  put_opt(body, "device_id", in.device_id);
  put_opt(body, "diff_mode", in.diff_mode);
  put_opt(body, "error_code", in.error_code);
  put_opt(body, "error_message", in.error_message);
  auto resp = execute_json("POST", project_url("telemetry/report"), body);
  if (resp.status != 202) throw_if_error(resp);
}

Bytes Client::media(const std::string& id, const DownloadOptions& opt) {
  std::string url = project_url("media/" + detail::url_encode(id));
  HeaderMap extra;
  extra["Accept"] = "application/octet-stream";
  if (opt.range) extra["Range"] = *opt.range;
  auto resp = execute("GET", url, extra);
  if (resp.status != 200 && resp.status != 206) throw_if_error(resp);
  return resp.body;
}

HttpResponse Client::head_media(const std::string& id) {
  auto resp = execute("HEAD", project_url("media/" + detail::url_encode(id)));
  throw_if_error(resp);
  return resp;
}

}  // namespace kirivers
