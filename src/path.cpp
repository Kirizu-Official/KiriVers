#include <kirivers/path.hpp>
#include <kirivers/adapters.hpp>

#include <cctype>
#include <filesystem>
#include <fstream>
#include <iterator>
#include <sstream>
#include <stdexcept>

namespace kirivers {

namespace {

bool has_control(std::string_view s) {
  for (unsigned char c : s) {
    if (c < 0x20) return true;
  }
  return false;
}

bool is_drive(std::string_view seg) {
  return seg.size() >= 2 && std::isalpha(static_cast<unsigned char>(seg[0])) != 0 &&
         seg[1] == ':';
}

}  // namespace

std::optional<std::string> normalize_rel_path(std::string_view raw, const NfcFn& nfc) {
  std::string trimmed(raw);
  auto b = trimmed.find_first_not_of(" \t\r\n");
  auto e = trimmed.find_last_not_of(" \t\r\n");
  if (b == std::string::npos) return std::nullopt;
  trimmed = trimmed.substr(b, e - b + 1);
  if (trimmed.empty() || has_control(trimmed)) return std::nullopt;
  if (trimmed.front() == '/' || trimmed.front() == '\\') return std::nullopt;
  if (is_drive(trimmed)) return std::nullopt;

  std::string slash;
  slash.reserve(trimmed.size());
  for (char c : trimmed) slash.push_back(c == '\\' ? '/' : c);

  {
    std::stringstream ss(slash);
    std::string seg;
    while (std::getline(ss, seg, '/')) {
      if (is_drive(seg) || seg == "." || seg == "..") return std::nullopt;
    }
  }

  if (nfc) slash = nfc(slash);

  std::string cleaned;
  cleaned.reserve(slash.size());
  bool prev_slash = false;
  for (char c : slash) {
    if (c == '/') {
      if (prev_slash) continue;
      prev_slash = true;
      cleaned.push_back('/');
    } else {
      prev_slash = false;
      cleaned.push_back(c);
    }
  }
  while (!cleaned.empty() && cleaned.front() == '/') cleaned.erase(cleaned.begin());
  while (!cleaned.empty() && cleaned.back() == '/') cleaned.pop_back();
  if (cleaned.empty()) return std::nullopt;

  {
    std::stringstream ss(cleaned);
    std::string seg;
    while (std::getline(ss, seg, '/')) {
      if (seg.empty() || seg == "." || seg == "..") return std::nullopt;
    }
  }
  return cleaned;
}

std::string to_lower_hex(const std::vector<unsigned char>& digest) {
  static const char* kHex = "0123456789abcdef";
  std::string out;
  out.resize(digest.size() * 2);
  for (size_t i = 0; i < digest.size(); ++i) {
    out[i * 2] = kHex[digest[i] >> 4];
    out[i * 2 + 1] = kHex[digest[i] & 0x0f];
  }
  return out;
}

std::string FunctionHasher::sha256_file(const std::string& path) const {
  std::ifstream in(std::filesystem::u8path(path), std::ios::binary);
  if (!in) throw std::runtime_error("cannot read file for hashing");
  Bytes data((std::istreambuf_iterator<char>(in)), std::istreambuf_iterator<char>());
  return sha256_hex(data);
}

bool equal_hex(std::string_view a, std::string_view b) {
  if (a.size() != b.size()) return false;
  for (size_t i = 0; i < a.size(); ++i) {
    if (std::tolower(static_cast<unsigned char>(a[i])) !=
        std::tolower(static_cast<unsigned char>(b[i]))) {
      return false;
    }
  }
  return true;
}

std::string build_check_payload(const std::string& version_integer,
                                const std::string& version_semver,
                                const std::string& root_hash,
                                const std::string& package_url,
                                const std::string& size,
                                const std::string& sha256_hex) {
  return version_integer + "\n" + version_semver + "\n" + root_hash + "\n" +
         package_url + "\n" + size + "\n" + sha256_hex;
}

}  // namespace kirivers
