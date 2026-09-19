#include "require.hpp"

#include <kirivers/kirivers.hpp>
#include <nlohmann/json.hpp>

#include <fstream>
#include <set>
#include <sstream>
#include <string>
#include <vector>

using namespace kirivers;

namespace {

HttpResponse json_resp(int status, const nlohmann::json& j,
                       const HeaderMap& extra = {}) {
  HttpResponse r;
  r.status = status;
  r.headers = extra;
  r.headers["Content-Type"] = "application/json";
  auto s = j.dump();
  r.body.assign(s.begin(), s.end());
  return r;
}

bool contains(const std::string& s, const char* part) {
  return s.find(part) != std::string::npos;
}

}  // namespace

int main() {
  std::ifstream in(std::string(KIRIVERS_SOURCE_DIR) + "/openapi.client.json");
  REQUIRE(in.good());
  nlohmann::json spec = nlohmann::json::parse(in);

  std::set<std::string> spec_ops;
  for (auto it = spec["paths"].begin(); it != spec["paths"].end(); ++it) {
    const std::string path = it.key();
    if (contains(path, "/store/") || path == "/api/v1/openapi.json") continue;
    static const std::set<std::string> verbs = {
        "get", "post", "put", "patch", "delete", "head", "trace"};
    for (auto mit = it.value().begin(); mit != it.value().end(); ++mit) {
      const std::string method = mit.key();
      if (!verbs.count(method)) continue;
      spec_ops.insert(method + " " + path);
    }
  }

  const std::set<std::string> sdk_ops = {
      "get /api/v1/health",
      "get /api/v1/projects/{project_ref}",
      "get /api/v1/projects/{project_ref}/announcements",
      "get /api/v1/projects/{project_ref}/channels",
      "get /api/v1/projects/{project_ref}/languages",
      "get /api/v1/projects/{project_ref}/matrix",
      "get /api/v1/projects/{project_ref}/media/{id}",
      "head /api/v1/projects/{project_ref}/media/{id}",
      "get /api/v1/projects/{project_ref}/packages/{ref}",
      "head /api/v1/projects/{project_ref}/packages/{ref}",
      "post /api/v1/projects/{project_ref}/telemetry/report",
      "post /api/v1/projects/{project_ref}/update/check",
      "post /api/v1/projects/{project_ref}/update/diff",
      "post /api/v1/projects/{project_ref}/update/pack",
      "get /api/v1/projects/{project_ref}/versions/{version}/integrity",
      "post /api/v1/projects/{project_ref}/clients/report",
      "get /api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}",
  };

  for (const auto& op : spec_ops) {
    REQUIRE(sdk_ops.count(op) == 1);
  }
  for (const auto& op : sdk_ops) {
    REQUIRE(spec_ops.count(op) == 1);
  }

  std::vector<HttpRequest> calls;
  auto handler = [&](const HttpRequest& req) -> HttpResponse {
    calls.push_back(req);
    if (contains(req.url, "/update/check")) {
      HeaderMap h{{"ETag", "\"abc\""}};
      if (req.method == "POST") {
        auto j = nlohmann::json::parse(bytes_to_string(req.body));
        REQUIRE(j.contains("current_version"));
        REQUIRE(j.contains("os"));
        REQUIRE(j.contains("arch"));
        REQUIRE(j.contains("capabilities"));
        REQUIRE(j["capabilities"] == nlohmann::json::array({"full_package"}));
        REQUIRE(!j.contains("accepted_delta_algos"));
        REQUIRE(!j.contains("local_sha256"));
        REQUIRE(!j.contains("dirty_paths"));
        REQUIRE(!j.contains("changelog"));
        REQUIRE(!j.contains("protocol_version"));
        if (header_value(req.headers, "If-None-Match") == "\"abc\"") {
          HttpResponse r;
          r.status = 304;
          r.headers = h;
          return r;
        }
        return json_resp(204, nlohmann::json::object(), h);
      }
    }
    if (contains(req.url, "/clients/report")) {
      return json_resp(200, {{"ip", "127.0.0.1"},
                             {"country_code", ""},
                             {"region_code", ""},
                             {"geo_i18n", nlohmann::json::object()}});
    }
    if (contains(req.url, "/update/diff")) {
      auto j = nlohmann::json::parse(bytes_to_string(req.body));
      REQUIRE(j.contains("source_version"));
      REQUIRE(j.contains("target_version"));
      REQUIRE(j.contains("os"));
      REQUIRE(j.contains("arch"));
      REQUIRE(j.contains("local_sha256"));
      return json_resp(200, {{"diff_mode", "full_package"},
                             {"root_hash", ""},
                             {"version_integer", nullptr},
                             {"version_semver", "1.1.0"},
                             {"channel", "stable"},
                             {"compare_engine", "semver"}});
    }
    if (contains(req.url, "/update/pack")) {
      auto j = nlohmann::json::parse(bytes_to_string(req.body));
      REQUIRE(j.contains("source_version"));
      REQUIRE(j.contains("needed_paths"));
      return json_resp(200, {{"status", "full_package"}});
    }
    if (contains(req.url, "/telemetry/report")) {
      REQUIRE(req.method == "POST");
      return json_resp(202, {{"status", "accepted"}});
    }
    if (contains(req.url, "/integrity")) {
      REQUIRE(contains(req.url, "os="));
      REQUIRE(contains(req.url, "arch="));
      return json_resp(200, {{"version_integer", nullptr},
                             {"version_semver", "1.1.0"},
                             {"channel", "stable"},
                             {"package_type", "single_file"},
                             {"root_hash", ""},
                             {"full_package_url", "/pkg"},
                             {"file_name", "a.bin"},
                             {"size", 1},
                             {"sha256", "aa"},
                             {"files", nlohmann::json::array()}});
    }
    if (contains(req.url, "/changelog/")) {
      return json_resp(200, {{"changelog", "hi"},
                             {"changelog_versions", nlohmann::json::array()}});
    }
    if (contains(req.url, "/announcements")) {
      return json_resp(200, {{"announcements", nlohmann::json::array()}});
    }
    if (contains(req.url, "/channels")) {
      return json_resp(200, {{"channels", nlohmann::json::array()}});
    }
    if (contains(req.url, "/languages")) {
      return json_resp(200, {{"languages", nlohmann::json::array()}});
    }
    if (contains(req.url, "/matrix")) {
      return json_resp(200, {{"matrix", nlohmann::json::array()}});
    }
    if (contains(req.url, "/health")) {
      return json_resp(200, {{"status", "ok"}, {"ready", true}});
    }
    if (contains(req.url, "/media/")) {
      HttpResponse r;
      r.status = 200;
      r.body = {'x'};
      return r;
    }
    if (contains(req.url, "/packages/")) {
      HttpResponse r;
      r.status = 200;
      r.body = {'p'};
      r.headers["Content-Type"] = "application/octet-stream";
      return r;
    }
    if (req.method == "GET" && contains(req.url, "/api/v1/projects/demo") &&
        !contains(req.url, "/api/v1/projects/demo/")) {
      return json_resp(200, {{"uuid", "u"},
                             {"slug", "demo"},
                             {"compare_engine", "semver"},
                             {"force_https", false},
                             {"require_client_token", false}});
    }
    HttpResponse r;
    r.status = 200;
    return r;
  };

  Config cfg;
  cfg.base_url = "http://example.test";
  cfg.project_ref = "demo";
  cfg.hosted_defaults = false;
  cfg.transport = std::make_shared<FunctionTransport>(handler);
  Client client(cfg);

  client.health();
  client.project();
  DeviceReportInput dr;
  dr.device_id = "dev-1";
  dr.os = "windows";
  dr.arch = "x86_64";
  client.report_device(dr);

  CheckInput cin;
  cin.current_version = "1.0.0";
  cin.os = "windows";
  cin.arch = "x86_64";
  cin.channel = "stable";
  auto cr = client.check(cin);
  REQUIRE(cr.status == CheckStatus::NoUpdate);
  CheckOptions none;
  none.if_none_match = "\"abc\"";
  auto cr304 = client.check(cin, none);
  REQUIRE(cr304.status == CheckStatus::NotModified);

  client.changelog("stable", "windows", "x86_64");
  IntegrityQuery iq;
  iq.os = "windows";
  iq.arch = "x86_64";
  client.integrity("1.1.0", iq);

  DiffInput din;
  din.source_version = "1.0.0";
  din.target_version = "1.1.0";
  din.os = "windows";
  din.arch = "x86_64";
  din.local_sha256 = "abc";
  client.diff(din);

  PackInput pin;
  pin.source_version = "1.0.0";
  pin.target_version = "1.1.0";
  pin.os = "windows";
  pin.arch = "x86_64";
  pin.needed_paths = {"bin\\app.exe", "bin/app.exe"};
  client.pack(pin);

  DownloadOptions range;
  range.range = "bytes=0-10";
  client.download("deadbeef", range);
  client.head_package("deadbeef");
  client.channels();
  client.matrix();
  client.languages();
  client.announcements();
  TelemetryInput tel;
  tel.os = "windows";
  tel.arch = "x86_64";
  tel.channel = "stable";
  tel.from_version = "1.0.0";
  tel.to_version = "1.1.0";
  tel.status = "installed";
  client.report_telemetry(tel);
  client.media("11111111-1111-1111-1111-111111111111");
  client.head_media("11111111-1111-1111-1111-111111111111");

  bool saw_check = false, saw_pack = false, saw_range = false, saw_exp = false;
  for (const auto& c : calls) {
    REQUIRE(!contains(c.url, "/store/"));
    REQUIRE(!contains(c.url, "/clients/login"));
    REQUIRE(!contains(c.url, "/update/pack/status"));
    REQUIRE(!contains(c.url, "/manifest"));
    REQUIRE(!contains(c.url, "/artifacts/"));
    REQUIRE(!contains(c.url, "/api/v1/ready"));
    if (c.method == "GET" && contains(c.url, "/update/check")) {
      REQUIRE(false);
    }
    if (c.method == "POST" && contains(c.url, "/update/check")) saw_check = true;
    if (contains(c.url, "/update/pack")) saw_pack = true;
    auto rng = header_value(c.headers, "Range");
    if (rng == "bytes=0-10") saw_range = true;
  }
  REQUIRE(saw_check);
  REQUIRE(saw_pack);
  REQUIRE(saw_range);

  auto signed_url = client.download(
      "http://cdn.example/file?exp=1&sig=abc");
  REQUIRE(!calls.empty());
  REQUIRE(contains(calls.back().url, "exp=1"));
  REQUIRE(contains(calls.back().url, "sig=abc"));
  (void)saw_exp;
  (void)signed_url;

  auto err_handler = [&](const HttpRequest&) {
    return json_resp(404, {{"error",
                            {{"code", "NOT_FOUND"},
                             {"message", "missing"},
                             {"details", nullptr}}}});
  };
  Config cfg2;
  cfg2.base_url = "http://example.test";
  cfg2.project_ref = "demo";
  cfg2.hosted_defaults = false;
  cfg2.transport = std::make_shared<FunctionTransport>(err_handler);
  Client c2(cfg2);
  bool threw = false;
  try {
    c2.project();
  } catch (const ApiError& e) {
    threw = true;
    REQUIRE(e.http_status() == 404);
    REQUIRE(e.code() == "NOT_FOUND");
    REQUIRE(std::string(e.what()) == "missing");
  }
  REQUIRE(threw);

  std::cout << "contract_test ok\n";
  return 0;
}
