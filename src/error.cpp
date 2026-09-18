#include <kirivers/error.hpp>

#include <sstream>

namespace kirivers {

ApiError::ApiError(int http_status, std::string code, std::string message,
                   nlohmann::json details)
    : std::runtime_error(std::move(message)),
      http_status_(http_status),
      code_(std::move(code)),
      details_(std::move(details)) {}

ApiError ApiError::from_body(int http_status, const std::string& body) {
  try {
    auto j = nlohmann::json::parse(body);
    if (j.contains("error") && j["error"].is_object()) {
      const auto& e = j["error"];
      std::string code = e.value("code", "UNKNOWN");
      std::string message = e.value("message", body);
      nlohmann::json details = nullptr;
      if (e.contains("details")) details = e["details"];
      return ApiError(http_status, std::move(code), std::move(message),
                      std::move(details));
    }
  } catch (const nlohmann::json::exception&) {
  }
  std::ostringstream os;
  os << "HTTP " << http_status;
  return ApiError(http_status, "HTTP_ERROR", os.str());
}

ConfigError::ConfigError(const std::string& what) : std::runtime_error(what) {}

}  // namespace kirivers
