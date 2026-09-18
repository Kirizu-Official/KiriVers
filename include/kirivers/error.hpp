#pragma once

#include <nlohmann/json.hpp>
#include <stdexcept>
#include <string>

namespace kirivers {

// Wire error envelope: { "error": { "code", "message", "details" } }.
class ApiError : public std::runtime_error {
 public:
  ApiError(int http_status, std::string code, std::string message,
           nlohmann::json details = nullptr);

  int http_status() const noexcept { return http_status_; }
  const std::string& code() const noexcept { return code_; }
  const nlohmann::json& details() const noexcept { return details_; }

  static ApiError from_body(int http_status, const std::string& body);

 private:
  int http_status_;
  std::string code_;
  nlohmann::json details_;
};

class ConfigError : public std::runtime_error {
 public:
  explicit ConfigError(const std::string& what);
};

}  // namespace kirivers
