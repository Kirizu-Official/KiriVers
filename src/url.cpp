#include "url.hpp"

#include <cctype>
#include <cstdio>
#include <algorithm>

namespace kirivers {
namespace detail {

std::string url_encode(std::string_view s) {
  std::string out;
  out.reserve(s.size());
  for (unsigned char c : s) {
    if (std::isalnum(c) != 0 || c == '-' || c == '_' || c == '.' || c == '~') {
      out.push_back(static_cast<char>(c));
    } else {
      char buf[4];
      std::snprintf(buf, sizeof(buf), "%%%02X", c);
      out.append(buf);
    }
  }
  return out;
}

std::string trim_slash(std::string s) {
  while (!s.empty() && s.back() == '/') s.pop_back();
  return s;
}

bool is_absolute_url(std::string_view s) {
  return s.find("://") != std::string_view::npos;
}

std::string join_url(std::string base, std::string path) {
  if (is_absolute_url(path)) return path;
  base = trim_slash(std::move(base));
  if (path.empty()) return base;
  if (path.front() != '/') path.insert(path.begin(), '/');
  return base + path;
}

std::string add_query(std::string url, const std::string& key, const std::string& val) {
  url.push_back(url.find('?') == std::string::npos ? '?' : '&');
  url += url_encode(key);
  url.push_back('=');
  url += url_encode(val);
  return url;
}

}  // namespace detail

std::string header_value(const HeaderMap& headers, const std::string& name) {
  auto ieq = [](const std::string& a, const std::string& b) {
    if (a.size() != b.size()) return false;
    for (size_t i = 0; i < a.size(); ++i) {
      if (std::tolower(static_cast<unsigned char>(a[i])) !=
          std::tolower(static_cast<unsigned char>(b[i]))) {
        return false;
      }
    }
    return true;
  };
  for (const auto& kv : headers) {
    if (ieq(kv.first, name)) return kv.second;
  }
  return {};
}

}  // namespace kirivers
