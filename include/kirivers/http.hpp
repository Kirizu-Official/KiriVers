#pragma once

#include <cstdint>
#include <map>
#include <string>
#include <vector>

namespace kirivers {

using Bytes = std::vector<std::uint8_t>;
using HeaderMap = std::map<std::string, std::string>;

struct HttpRequest {
  std::string method;
  std::string url;
  HeaderMap headers;
  Bytes body;
};

struct HttpResponse {
  int status = 0;
  HeaderMap headers;
  Bytes body;
};

// Case-insensitive header lookup. Empty if missing.
std::string header_value(const HeaderMap& headers, const std::string& name);

inline std::string bytes_to_string(const Bytes& b) {
  return std::string(reinterpret_cast<const char*>(b.data()), b.size());
}

inline Bytes string_to_bytes(const std::string& s) {
  return Bytes(s.begin(), s.end());
}

}  // namespace kirivers
