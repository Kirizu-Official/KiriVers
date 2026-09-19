#include "require.hpp"

#include <kirivers/kirivers.hpp>
#include <nlohmann/json.hpp>

#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <random>
#include <sstream>
#include <string>

using namespace kirivers;
namespace fs = std::filesystem;

namespace {

std::string env_or(const char* key, const std::string& fallback) {
  const char* v = std::getenv(key);
  return (v && *v) ? std::string(v) : fallback;
}

std::optional<fs::path> find_fixture() {
  if (const char* p = std::getenv("KIRIVERS_FIXTURE")) {
    fs::path x(p);
    if (fs::exists(x)) return x;
  }
  const fs::path candidates[] = {
      fs::path("/fixture.json"),
      fs::path("D:/KiriVers/configs/sdk-fixture.json"),
      fs::path("D:\\KiriVers\\configs\\sdk-fixture.json"),
      fs::path(KIRIVERS_SOURCE_DIR) / "sdk-fixture.json",
  };
  for (const auto& c : candidates) {
    if (fs::exists(c)) return c;
  }
  return std::nullopt;
}

void write_backend_issue(const std::string& repro, const std::string& expected,
                         const std::string& actual) {
  const fs::path out = fs::path(KIRIVERS_SOURCE_DIR) / "BACKEND_ISSUE.md";
  std::ofstream f(out);
  f << "# Backend issue (C++ SDK integration)\n\n"
    << "## Repro\n\n"
    << repro << "\n\n"
    << "## Expected\n\n"
    << expected << "\n\n"
    << "## Actual\n\n"
    << actual << "\n\n"
    << "## Suggested fix\n\n"
    << "Fix the live client plane on :8080 so POST /update/check for "
       "sdk-fixture from 1.0.0 windows/x86_64 returns 200 with package 1.1.0 "
       "sha256 7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969. "
       "Do not change SDK contract to match a broken server.\n";
}

std::string random_device_suffix() {
  std::random_device rd;
  std::mt19937_64 gen(rd());
  std::ostringstream os;
  os << std::hex << gen() << gen();
  return os.str();
}

}  // namespace

int main() {
  if (const char* skip = std::getenv("KIRIVERS_SKIP_INTEGRATION")) {
    if (skip[0] == '1') {
      std::cerr << "integration skipped via KIRIVERS_SKIP_INTEGRATION\n";
      return 0;
    }
  }
  auto fixture_path = find_fixture();
  if (!fixture_path) {
    std::cerr << "sdk-fixture.json not found; set KIRIVERS_FIXTURE\n";
    return 1;
  }
  std::ifstream in(*fixture_path);
  nlohmann::json fixture = nlohmann::json::parse(in);

  const std::string base =
      env_or("KIRIVERS_CLIENT_BASE_URL", fixture.value("client_base_url",
                                                       "http://127.0.0.1:8080"));
  const std::string project = fixture.value("project_ref", "sdk-fixture");
  const std::string channel = fixture.value("channel", "stable");
  const std::string os = fixture.value("os", "windows");
  const std::string arch = fixture.value("arch", "x86_64");
  const std::string current = fixture.value("current_version", "1.0.0");
  const std::string target = fixture.value("target_version", "1.1.0");
  const std::string expect_sha =
      fixture["sha256"].value(target,
                              "7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969");

  Config cfg;
  cfg.base_url = base;
  cfg.project_ref = project;
  cfg.hosted_defaults = true;

  Client client(cfg);
  try {
    auto h = client.health();
    REQUIRE(h.status == "ok");
  } catch (const std::exception& ex) {
    std::cerr << "client plane not reachable at " << base << ": " << ex.what()
              << "\n";
    return 1;
  }

  const std::string device_id = "sdk-cpp-" + random_device_suffix();

  CheckInput cin;
  cin.current_version = current;
  cin.os = os;
  cin.arch = arch;
  cin.channel = channel;
  cin.device_id = device_id;
  cin.capabilities = std::vector<std::string>{"full_package"};

  CheckResult checked;
  try {
    checked = client.check(cin);
  } catch (const ApiError& e) {
    write_backend_issue(
        "POST " + base + "/api/v1/projects/" + project +
            "/update/check with current_version=1.0.0 os=windows arch=x86_64 "
            "channel=stable capabilities=[full_package] (unique sdk-cpp device_id).",
        "HTTP 200 UpdateCheck200 targeting 1.1.0 with sha256 " + expect_sha,
        "ApiError " + e.code() + " HTTP " + std::to_string(e.http_status()) +
            ": " + e.what());
    std::cerr << "check failed; wrote BACKEND_ISSUE.md\n";
    return 1;
  }

  if (checked.status != CheckStatus::Update || !checked.body) {
    write_backend_issue(
        "POST check from 1.0.0 on sdk-fixture windows/x86_64.",
        "HTTP 200 with has_update, version_semver=1.1.0, matching sha256.",
        checked.status == CheckStatus::NoUpdate
            ? "HTTP 204 NoUpdate"
            : "HTTP 304 or empty body");
    std::cerr << "unexpected check status; wrote BACKEND_ISSUE.md\n";
    return 1;
  }

  const auto& body = *checked.body;
  if (body.version_semver != target || body.sha256 != expect_sha) {
    write_backend_issue(
        "POST check from 1.0.0 on sdk-fixture windows/x86_64.",
        "version_semver=" + target + " sha256=" + expect_sha,
        "version_semver=" + body.version_semver.value_or("<null>") +
            " sha256=" + body.sha256);
    std::cerr << "check payload mismatch; wrote BACKEND_ISSUE.md\n";
    return 1;
  }

  Bytes pkg;
  try {
    pkg = client.download(body.package_url);
  } catch (const std::exception& ex) {
    write_backend_issue(
        "GET " + body.package_url,
        "200 octet-stream whose SHA-256 is " + expect_sha,
        std::string("download failed: ") + ex.what());
    std::cerr << "download failed; wrote BACKEND_ISSUE.md\n";
    return 1;
  }

  auto hasher = client.hasher();
  REQUIRE(hasher != nullptr);
  const std::string got = hasher->sha256_hex(pkg);
  if (!equal_hex(got, expect_sha)) {
    write_backend_issue(
        "GET package_url from check and SHA-256 the bytes.",
        expect_sha, got);
    std::cerr << "sha256 mismatch; wrote BACKEND_ISSUE.md\n";
    return 1;
  }

  const fs::path stage = fs::temp_directory_path() /
                         ("kirivers-sdk-cpp-" + random_device_suffix() + ".bin");
  {
    std::ofstream out(stage, std::ios::binary);
    out.write(reinterpret_cast<const char*>(pkg.data()),
              static_cast<std::streamsize>(pkg.size()));
  }

  Updater updater(client);
  UpdateRequest ureq;
  ureq.current_version = current;
  ureq.os = os;
  ureq.arch = arch;
  ureq.channel = channel;
  ureq.device_id = device_id;
  ureq.stage_path = stage.string();
  auto result = updater.run(ureq);
  REQUIRE(result.kind == UpdateKind::Staged || result.kind == UpdateKind::NoUpdate ||
          result.kind == UpdateKind::NotModified);
  if (result.kind == UpdateKind::Staged) {
    REQUIRE(equal_hex(result.sha256, expect_sha));
  }

  std::cout << "integration_test ok (check 1.0.0 -> 1.1.0, sha256 verified)\n";
  return 0;
}
