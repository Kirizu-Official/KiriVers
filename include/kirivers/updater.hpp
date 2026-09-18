#pragma once

#include "client.hpp"

#include <optional>
#include <string>

namespace kirivers {

struct UpdateRequest {
  std::string current_version;
  std::string os;
  std::string arch;
  std::string channel = "stable";
  std::optional<std::string> device_id;
  std::optional<std::string> os_version;
  std::optional<std::string> hw_rev;
  std::string stage_path;
  std::string install_dir;
  bool report_device = false;
  nlohmann::json device_custom = nullptr;
};

enum class UpdateKind { NoUpdate, NotModified, Staged, Applied };

struct UpdateResult {
  UpdateKind kind = UpdateKind::NoUpdate;
  CheckResult check;
  std::string staged_path;
  std::string sha256;
  std::string diff_mode = "full_package";
  std::optional<std::string> applied_path;
};

class Updater {
 public:
  explicit Updater(Client& client) : client_(client) {}

  UpdateResult run(const UpdateRequest& req);

 private:
  Bytes download_and_verify(const std::string& url, const std::string& expect_sha,
                            const std::optional<std::string>& signature,
                            const UpdateCheckBody* check);
  void maybe_verify_signature(const UpdateCheckBody& body);
  void write_stage(const std::string& path, const Bytes& data);
  void send_telemetry(const UpdateRequest& req, const std::string& to_version,
                      const std::string& status, const std::string& diff_mode,
                      const std::string& err_code = {},
                      const std::string& err_msg = {});

  Client& client_;
};

}  // namespace kirivers
