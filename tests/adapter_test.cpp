#include "require.hpp"

#include "delta.hpp"
#include <kirivers/kirivers.hpp>

#include <nlohmann/json.hpp>
#include <chrono>
#include <map>
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
    REQUIRE(equal_hex("AaBb", "aabb"));
    REQUIRE(!equal_hex("aa", "ab"));
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
    Bytes hd{'H', 'D', 'I', 'F', 'F', '1', '3', '&'};
    REQUIRE(detect_delta_magic(hd) == DeltaMagic::Hdiff13);
    REQUIRE(magic_matches_algo(DeltaMagic::Hdiff13, "hdiffpatch"));
    REQUIRE(!magic_matches_algo(DeltaMagic::Hdiff13, "xdelta3"));
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

    CheckInput empty_caps = in;
    empty_caps.capabilities = std::vector<std::string>{};
    client.check(empty_caps);
    auto j_empty = nlohmann::json::parse(bytes_to_string(calls.back().body));
    REQUIRE(j_empty["capabilities"] == nlohmann::json::array({"full_package"}));
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

  {
    const std::string sha(64, 'a');
    class MemStore final : public FileStore {
     public:
      std::map<std::string, Bytes> files;
      Bytes read_file(const std::string& p) const override {
        auto it = files.find(p);
        if (it == files.end()) throw std::runtime_error("missing");
        return it->second;
      }
      void write_file(const std::string& p, const Bytes& d) override { files[p] = d; }
      bool exists(const std::string& p) const override { return files.count(p) != 0; }
      std::vector<std::string> list_files(const std::string&) const override { return {}; }
    };
    auto store = std::make_shared<MemStore>();
    int patch_apply = 0;
    auto h = [&](const HttpRequest& req) -> HttpResponse {
      HttpResponse r;
      r.status = 200;
      auto json_body = [&](const nlohmann::json& j) {
        r.headers["Content-Type"] = "application/json";
        auto s = j.dump();
        r.body.assign(s.begin(), s.end());
        return r;
      };
      if (req.url.find("/update/check") != std::string::npos) {
        return json_body({{"has_update", true},
                          {"is_downgrade", false},
                          {"compare_engine", "semver"},
                          {"version_semver", "1.1.0"},
                          {"target_channel", "stable"},
                          {"package_type", "single_file"},
                          {"root_hash", ""},
                          {"package_url", "http://example.test/full.bin"},
                          {"file_name", "app.bin"},
                          {"size", 4},
                          {"sha256", sha},
                          {"delta_available", true},
                          {"delta_algo", "bsdiff"}});
      }
      if (req.url.find("/update/diff") != std::string::npos) {
        return json_body({{"diff_mode", "binary_delta"},
                          {"root_hash", ""},
                          {"version_semver", "1.1.0"},
                          {"channel", "stable"},
                          {"compare_engine", "semver"},
                          {"package_url", "http://example.test/delta.bin"},
                          {"sha256", sha},
                          {"delta_algo", "bsdiff"}});
      }
      if (req.url.find("/delta.bin") != std::string::npos) {
        r.body = {'X', 'X', 'X'};
        return r;
      }
      if (req.url.find("/full.bin") != std::string::npos) {
        r.body = {'F', 'U', 'L', 'L'};
        return r;
      }
      if (req.url.find("/telemetry/report") != std::string::npos) {
        r.status = 202;
        return r;
      }
      return r;
    };
    Config cfg;
    cfg.base_url = "http://example.test";
    cfg.project_ref = "p";
    cfg.hosted_defaults = false;
    cfg.transport = std::make_shared<FunctionTransport>(h);
    cfg.file_store = store;
    cfg.hasher = std::make_shared<FunctionHasher>([&](const Bytes&) { return sha; });
    cfg.patcher = std::make_shared<FunctionPatcher>(
        std::vector<std::string>{"bsdiff"},
        [&](const Bytes&, const Bytes& d, const std::string&) {
          ++patch_apply;
          return d;
        });
    Client client(cfg);
    Updater updater(client);
    UpdateRequest ureq;
    ureq.current_version = "1.0.0";
    ureq.os = "linux";
    ureq.arch = "x86_64";
    ureq.stage_path = "staged.bin";
    auto result = updater.run(ureq);
    REQUIRE(result.kind == UpdateKind::Staged);
    REQUIRE(result.diff_mode == "full_package");
    REQUIRE(patch_apply == 0);
    REQUIRE(store->files["staged.bin"] == Bytes({'F', 'U', 'L', 'L'}));
  }

  {
    const std::string sha(64, 'b');
    class MemStore final : public FileStore {
     public:
      std::map<std::string, Bytes> files;
      Bytes read_file(const std::string& p) const override {
        auto it = files.find(p);
        if (it == files.end()) throw std::runtime_error("missing");
        return it->second;
      }
      void write_file(const std::string& p, const Bytes& d) override { files[p] = d; }
      bool exists(const std::string& p) const override { return files.count(p) != 0; }
      std::vector<std::string> list_files(const std::string&) const override { return {}; }
    };
    class RecUnpack final : public ArchiveUnpacker {
     public:
      int calls = 0;
      std::string dest;
      void unpack_zip(const Bytes&, FileStore&, const std::string& dest_root,
                      const std::vector<std::pair<std::string, std::string>>& m) override {
        ++calls;
        dest = dest_root;
        REQUIRE(!m.empty());
        REQUIRE(m.front().second == "bin/app");
      }
    };
    auto store = std::make_shared<MemStore>();
    auto unpack = std::make_shared<RecUnpack>();
    auto h = [&](const HttpRequest& req) -> HttpResponse {
      HttpResponse r;
      r.status = 200;
      auto json_body = [&](const nlohmann::json& j) {
        r.headers["Content-Type"] = "application/json";
        auto s = j.dump();
        r.body.assign(s.begin(), s.end());
        return r;
      };
      if (req.url.find("/update/check") != std::string::npos) {
        return json_body({{"has_update", true},
                          {"is_downgrade", false},
                          {"compare_engine", "semver"},
                          {"version_semver", "1.1.0"},
                          {"target_channel", "stable"},
                          {"package_type", "multi_file"},
                          {"root_hash", ""},
                          {"package_url", "http://example.test/full.zip"},
                          {"file_name", "app.zip"},
                          {"size", 1},
                          {"sha256", sha},
                          {"delta_available", false}});
      }
      if (req.url.find("/integrity") != std::string::npos) {
        return json_body(
            {{"version_semver", "1.1.0"},
             {"channel", "stable"},
             {"package_type", "multi_file"},
             {"root_hash", ""},
             {"full_package_url", "http://example.test/full.zip"},
             {"file_name", "app.zip"},
             {"size", 1},
             {"sha256", sha},
             {"files", nlohmann::json::array(
                           {{{"path", "bin/app"},
                             {"size", 1},
                             {"install_policy", "REPLACE"},
                             {"integrity_check", true},
                             {"sha256", sha}}})}});
      }
      if (req.url.find("/update/pack") != std::string::npos) {
        return json_body({{"status", "ready"},
                          {"diff_mode", "patch_package"},
                          {"package_url", "http://example.test/patch.zip"},
                          {"sha256", sha},
                          {"files", nlohmann::json::array(
                                        {{{"path", "bin/app"}, {"sha256", sha}}})}});
      }
      if (req.url.find("/patch.zip") != std::string::npos) {
        r.body = {'Z', 'I', 'P'};
        return r;
      }
      if (req.url.find("/telemetry/report") != std::string::npos) {
        r.status = 202;
        return r;
      }
      return r;
    };
    Config cfg;
    cfg.base_url = "http://example.test";
    cfg.project_ref = "p";
    cfg.hosted_defaults = false;
    cfg.transport = std::make_shared<FunctionTransport>(h);
    cfg.file_store = store;
    cfg.archive_unpacker = unpack;
    cfg.hasher = std::make_shared<FunctionHasher>([&](const Bytes&) { return sha; });
    Client client(cfg);
    Updater updater(client);
    UpdateRequest ureq;
    ureq.current_version = "1.0.0";
    ureq.os = "linux";
    ureq.arch = "x86_64";
    ureq.install_dir = "app";
    ureq.stage_path = "staged.zip";
    auto result = updater.run(ureq);
    REQUIRE(result.kind == UpdateKind::Staged);
    REQUIRE(result.diff_mode == "patch_package");
    REQUIRE(unpack->calls == 1);
    REQUIRE(unpack->dest == "app");
    REQUIRE(store->files["staged.zip"] == Bytes({'Z', 'I', 'P'}));
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
