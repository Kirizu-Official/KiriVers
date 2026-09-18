#pragma once

#include "adapters.hpp"
#include "error.hpp"
#include "types.hpp"

#include <chrono>
#include <memory>
#include <string>
#include <vector>

namespace kirivers {

struct Config {
  std::string base_url;
  std::string project_ref;
  std::string project_token;
  std::string channel_token;
  std::string public_key_pem;
  std::string signing_algo;  // "ed25519" or "rsa-sha256" when verifying signatures

  std::shared_ptr<Transport> transport;
  std::shared_ptr<FileStore> file_store;
  std::shared_ptr<Hasher> hasher;
  std::shared_ptr<Patcher> patcher;
  std::shared_ptr<Replacer> replacer;
  std::shared_ptr<ArchiveUnpacker> archive_unpacker;
  std::shared_ptr<SignatureVerifier> signature_verifier;

  // Hosted builds fill null adapters with libcurl/OpenSSL/libzip/utf8proc.
  // Tests and embedded callers set this false and inject Transport.
  bool hosted_defaults = true;
};

struct PackWaitOptions {
  std::chrono::milliseconds initial{1000};
  std::chrono::milliseconds cap{15000};
  std::chrono::milliseconds deadline{120000};
};

class Client {
 public:
  explicit Client(Config cfg);

  const Config& config() const { return cfg_; }
  DeclaredCapabilities declared_capabilities() const;

  HealthStatus health();
  ProjectPublic project();
  DeviceReportOutput report_device(const DeviceReportInput& in);
  CheckResult check(const CheckInput& in, const CheckOptions& opt = {});
  ChangelogBody changelog(const std::string& channel, const std::string& os,
                          const std::string& arch, const ChangelogQuery& q = {});
  IntegrityBody integrity(const std::string& version, const IntegrityQuery& q);
  DiffBody diff(const DiffInput& in);
  PackBody pack(const PackInput& in);
  PackBody pack_wait(const PackInput& in, const PackWaitOptions& opt = {});
  Bytes download(const std::string& url_or_sha256, const DownloadOptions& opt = {});
  HttpResponse head_package(const std::string& url_or_sha256,
                            const DownloadOptions& opt = {});
  std::vector<ChannelRow> channels();
  std::vector<MatrixRow> matrix();
  std::vector<LanguageRow> languages();
  AnnouncementList announcements(const AnnouncementQuery& q = {});
  void report_telemetry(const TelemetryInput& in);
  Bytes media(const std::string& id, const DownloadOptions& opt = {});
  HttpResponse head_media(const std::string& id);

  std::shared_ptr<Transport> transport() const { return cfg_.transport; }
  std::shared_ptr<FileStore> file_store() const { return cfg_.file_store; }
  std::shared_ptr<Hasher> hasher() const { return cfg_.hasher; }
  std::shared_ptr<Patcher> patcher() const { return cfg_.patcher; }
  std::shared_ptr<Replacer> replacer() const { return cfg_.replacer; }
  std::shared_ptr<ArchiveUnpacker> archive_unpacker() const {
    return cfg_.archive_unpacker;
  }
  std::shared_ptr<SignatureVerifier> signature_verifier() const {
    return cfg_.signature_verifier;
  }

 private:
  friend class Updater;

  HttpResponse execute(const std::string& method, const std::string& url,
                       const HeaderMap& extra = {}, const Bytes& body = {},
                       const std::string& content_type = {});
  HttpResponse execute_json(const std::string& method, const std::string& url,
                            const nlohmann::json& body);
  void apply_auth(HeaderMap& h) const;
  std::string project_url(const std::string& suffix) const;
  std::string resolve_download_url(const std::string& url_or_sha256) const;
  nlohmann::json parse_success_json(const HttpResponse& resp) const;
  void throw_if_error(const HttpResponse& resp) const;

  Config cfg_;
};

}  // namespace kirivers
