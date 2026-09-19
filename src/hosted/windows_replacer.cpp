#include <kirivers/hosted.hpp>
#include <kirivers/error.hpp>

#ifdef _WIN32
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#include <windows.h>
#include <string>
#include <vector>

namespace kirivers {

namespace {

std::wstring utf8_to_wide(const std::string& s) {
  if (s.empty()) return {};
  const int n = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, s.data(),
                                    static_cast<int>(s.size()), nullptr, 0);
  if (n <= 0) throw std::runtime_error("utf8_to_wide failed");
  std::wstring w(static_cast<size_t>(n), L'\0');
  MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, s.data(),
                      static_cast<int>(s.size()), w.data(), n);
  return w;
}

}  // namespace

void MoveFileExReplacer::replace(const std::string& staged_path,
                                 const std::string& dest_path) {
  const std::wstring src = utf8_to_wide(staged_path);
  const std::wstring dst = utf8_to_wide(dest_path);
  const DWORD flags =
      MOVEFILE_REPLACE_EXISTING | MOVEFILE_COPY_ALLOWED | MOVEFILE_WRITE_THROUGH;
  if (MoveFileExW(src.c_str(), dst.c_str(), flags)) return;
  throw std::runtime_error("MoveFileExW failed, last error " +
                           std::to_string(GetLastError()));
}

}  // namespace kirivers
#endif
