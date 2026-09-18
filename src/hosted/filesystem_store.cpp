#include <kirivers/hosted.hpp>
#include <kirivers/error.hpp>

#include <utf8proc.h>
#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <iterator>
#include <stdexcept>

namespace kirivers {

namespace fs = std::filesystem;

std::string utf8proc_nfc(std::string_view s) {
  utf8proc_uint8_t* out = nullptr;
  const utf8proc_ssize_t n = utf8proc_map(
      reinterpret_cast<const utf8proc_uint8_t*>(s.data()),
      static_cast<utf8proc_ssize_t>(s.size()), &out,
      static_cast<utf8proc_option_t>(UTF8PROC_STABLE | UTF8PROC_COMPOSE));
  if (n < 0 || !out) {
    throw std::runtime_error(utf8proc_errmsg(n));
  }
  std::string r(reinterpret_cast<char*>(out), static_cast<size_t>(n));
  std::free(out);
  return r;
}

FilesystemStore::FilesystemStore(std::string root) : root_(std::move(root)) {}

std::string FilesystemStore::normalize_path(std::string_view raw) const {
  auto n = normalize_rel_path(raw, utf8proc_nfc);
  if (!n) throw std::invalid_argument("invalid relative path");
  return *n;
}

Bytes FilesystemStore::read_file(const std::string& path) const {
  fs::path p = fs::u8path(root_) / fs::u8path(path);
  std::ifstream in(p, std::ios::binary);
  if (!in) throw std::runtime_error("read_file failed");
  return Bytes((std::istreambuf_iterator<char>(in)), std::istreambuf_iterator<char>());
}

void FilesystemStore::write_file(const std::string& path, const Bytes& data) {
  fs::path p = fs::u8path(root_) / fs::u8path(path);
  if (p.has_parent_path()) fs::create_directories(p.parent_path());
  std::ofstream out(p, std::ios::binary | std::ios::trunc);
  if (!out) throw std::runtime_error("write_file failed");
  out.write(reinterpret_cast<const char*>(data.data()),
            static_cast<std::streamsize>(data.size()));
}

bool FilesystemStore::exists(const std::string& path) const {
  return fs::exists(fs::u8path(root_) / fs::u8path(path));
}

std::vector<std::string> FilesystemStore::list_files(const std::string& dir) const {
  std::vector<std::string> out;
  fs::path base = fs::u8path(root_) / fs::u8path(dir);
  if (!fs::exists(base)) return out;
  for (const auto& entry : fs::recursive_directory_iterator(base)) {
    if (!entry.is_regular_file()) continue;
    out.push_back(fs::relative(entry.path(), base).generic_string());
  }
  return out;
}

}  // namespace kirivers
