#pragma once

#include <kirivers/http.hpp>

#include <string>
#include <string_view>
#include <utility>
#include <vector>

namespace kirivers {
namespace detail {

std::string url_encode(std::string_view s);
std::string join_url(std::string base, std::string path);
std::string add_query(std::string url, const std::string& key, const std::string& val);
std::string trim_slash(std::string s);
bool is_absolute_url(std::string_view s);

}  // namespace detail
}  // namespace kirivers
