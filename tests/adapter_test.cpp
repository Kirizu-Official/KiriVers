#include "require.hpp"

#include "delta.hpp"
#include <kirivers/kirivers.hpp>

#include <nlohmann/json.hpp>
#include <chrono>
#include <string>
#include <vector>

using namespace kirivers;

int main() {
  {
    auto a = normalize_rel_path(R"(bin\resources\config.json)");
    REQUIRE(a.has_value());
    REQUIRE(*a == "bin/resources/config.json");
    auto b = normalize_rel_path("bin//sub\\\\dir///file.txt");
    REQUIRE(b.has_value());
    REQUIRE(*b == "bin/sub/dir/file.txt");
    REQUIRE(!normalize_rel_path("../etc/passwd"));
    REQUIRE(!normalize_rel_path("/abs"));
    REQUIRE(!normalize_rel_path("C:foo"));
  }

#if defined(KIRIVERS_HOSTED)
  {
    const std::string nfd = "caf\u0065\u0301.txt";
    const std::string nfc = "caf\u00e9.txt";
    auto a = normalize_rel_path(nfd, utf8proc_nfc);
    auto b = normalize_rel_path(nfc, utf8proc_nfc);
    REQUIRE(a.has_value());
    REQUIRE(b.has_value());
    REQUIRE(*a == *b);
  }
#endif

  {
    Bytes kv{'K', 'V', 'D', 'I', 'F', 'F', 'H', 'P', '1', '\n'};
    REQUIRE(detect_delta_magic(kv) == DeltaMagic::KvDiffHp1);
    REQUIRE(magic_matches_algo(DeltaMagic::KvDiffHp1, "hdiffpatch"));
    REQUIRE(!magic_matches_algo(DeltaMagic::KvDiffHp1, "bsdiff"));
    Bytes bs{'B', 'S', 'D', 'I', 'F', 'F', '4', '0'};
    REQUIRE(detect_delta_magic(bs) == DeltaMagic::Bsdiff40);
    Bytes vc{static_cast<unsigned char>(0xD6), static_cast<unsigned char>(0xC3),
             static_cast<unsigned char>(0xC4)};
    REQUIRE(detect_delta_magic(vc) == DeltaMagic::Vcdiff);
    Bytes unk{'X'};
    REQUIRE(detect_delta_magic(unk) == DeltaMagic::Unknown);
  }

  std::vector<HttpRequest> calls;
  int pack_hits = 0;
  std::string first_pack_body;
  auto handler = [&](const HttpRequest& req) -> HttpResponse {
    calls.push_back(req);
    HttpResponse r;
    r.status = 200;
    if (req.url.find("/update/check") != std::string::npos) {
      auto j = nlohmann::json::parse(bytes_to_string(req.body));
      r.headers["Content-Type"] = "application/json";
      auto body = j.dump();
      r.body.assign(body.begin(), body.end());
      r.status = 204;
      return r;
    }
    if (req.url.find("/update/pack") != std::string::npos) {
      ++pack_hits;
      const std::string b = bytes_to_string(req.body);
      if (pack_hits == 1) first_pack_body = b;
      REQUIRE(b == first_pack_body);
      nlohmann::json out;
      if (pack_hits < 2) {
        r.status = 202;
        out = {{"status", "pending"}};
      } else {
        r.status = 200;
        out = {{"status", "ready"},
               {"package_url", "/p"},
               {"diff_mode", "patch_package"}};
      }
      auto s = out.dump();
      r.body.assign(s.begin(), s.end());
      return r;
    }
    return r;
  };

  {
    Config cfg;
    cfg.base_url = "http://example.test";
    cfg.project_ref = "p";
    cfg.hosted_defaults = false;
    cfg.transport = std::make_shared<FunctionTransport>(handler);
    Client client(cfg);
    auto caps = client.declared_capabilities();
    REQUIRE(caps.capabilities.size() == 1);
    REQUIRE(caps.capabilities[0] == "full_package");
    REQUIRE(caps.accepted_delta_algos.empty());

    CheckInput in;
    in.current_version = "1.0.0";
    in.os = "linux";
    in.arch = "x86_64";
    client.check(in);
    auto j = nlohmann::json::parse(bytes_to_string(calls.back().body));
    REQUIRE(j["capabilities"] == nlohmann::json::array({"full_package"}));
    REQUIRE(!j.contains("accepted_delta_algos"));
  }

  {
    calls.clear();
    Config cfg;
    cfg.base_url = "http://example.test";
    cfg.project_ref = "p";
    cfg.hosted_defaults = false;
    cfg.transport = std::make_shared<FunctionTransport>(handler);
    cfg.patcher = std::make_shared<FunctionPatcher>(
        std::vector<std::string>{"bsdiff", "xdelta3", "hdiffpatch"},
        [](const Bytes&, const Bytes& d, const std::string&) { return d; });
    class DummyUnpack final : public ArchiveUnpacker {
     public:
      void unpack_zip(const Bytes&, FileStore&, const std::string&,
                      const std::vector<std::pair<std::string, std::string>>&)
          override {}
    };
    class DummyStore final : public FileStore {
     public:
      Bytes read_file(const std::string&) const override { return {}; }
      void write_file(const std::string&, const Bytes&) override {}
      bool exists(const std::string&) const override { return false; }
      std::vector<std::string> list_files(const std::string&) const override {
        return {};
      }
      bool can_write_individual_files() const override { return true; }
    };
    cfg.archive_unpacker = std::make_shared<DummyUnpack>();
    cfg.file_store = std::make_shared<DummyStore>();
    Client client(cfg);
    auto caps = client.declared_capabilities();
    bool has_delta = false, has_patch = false, has_files = false;
    for (const auto& c : caps.capabilities) {
      if (c == "binary_delta") has_delta = true;
      if (c == "patch_package") has_patch = true;
      if (c == "file_list") has_files = true;
    }
    REQUIRE(has_delta);
    REQUIRE(has_patch);
    REQUIRE(has_files);
    REQUIRE(!caps.accepted_delta_algos.empty());

    CheckInput in;
    in.current_version = "1.0.0";
    in.os = "linux";
    in.arch = "x86_64";
    client.check(in);
    auto j = nlohmann::json::parse(bytes_to_string(calls.back().body));
    REQUIRE(j.contains("accepted_delta_algos"));
    REQUIRE(j["accepted_delta_algos"].size() == 3);
  }

  {
    pack_hits = 0;
    first_pack_body.clear();
    Config cfg;
    cfg.base_url = "http://example.test";
    cfg.project_ref = "p";
    cfg.hosted_defaults = false;
    cfg.transport = std::make_shared<FunctionTransport>(handler);
    Client client(cfg);
    PackInput pin;
    pin.source_version = "1.0.0";
    pin.target_version = "1.1.0";
    pin.os = "linux";
    pin.arch = "x86_64";
    pin.needed_paths = {"a/b", "a\\b"};
    PackWaitOptions opt;
    opt.initial = std::chrono::milliseconds(1);
    opt.cap = std::chrono::milliseconds(1);
    opt.deadline = std::chrono::milliseconds(2000);
    auto packed = client.pack_wait(pin, opt);
    REQUIRE(packed.status == "ready");
    REQUIRE(pack_hits == 2);
  }

#if defined(KIRIVERS_HOSTED)
  {
    calls.clear();
    Config cfg;
    cfg.base_url = "http://example.test";
    cfg.project_ref = "p";
    cfg.hosted_defaults = true;
    cfg.transport = std::make_shared<FunctionTransport>(handler);
    Client client(cfg);
    auto caps = client.declared_capabilities();
    bool has_full = false, has_delta = false, has_patch = false, has_files = false;
    for (const auto& c : caps.capabilities) {
      if (c == "full_package") has_full = true;
      if (c == "binary_delta") has_delta = true;
      if (c == "patch_package") has_patch = true;
      if (c == "file_list") has_files = true;
    }
    REQUIRE(has_full);
    REQUIRE(has_patch);
    REQUIRE(has_files);
    REQUIRE(!has_delta);
    REQUIRE(caps.accepted_delta_algos.empty());
    REQUIRE(client.replacer() == nullptr);
  }
#endif

#if defined(KIRIVERS_EMBEDDED)
  {
    Config cfg;
    cfg.base_url = "http://example.test";
    cfg.project_ref = "p";
    bool threw = false;
    try {
      Client client(cfg);
      (void)client;
    } catch (const ConfigError&) {
      threw = true;
    }
    REQUIRE(threw);
  }
#endif

  std::cout << "adapter_test ok\n";
  return 0;
}
