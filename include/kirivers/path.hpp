#pragma once

#include <functional>
#include <optional>
#include <string>
#include <string_view>
#include <vector>

namespace kirivers {

using NfcFn = std::function<std::string(std::string_view)>;

// Relative-path rules matching server pkg/pathutil:
// reject NUL/control, drive letters, leading slash, `.` / `..`; `\` → `/`;
// collapse duplicate slashes. When nfc is set (hosted FileStore uses utf8proc)
// the result is NFC; embedded builds leave NFC to the injected FileStore.
std::optional<std::string> normalize_rel_path(std::string_view raw,
                                              const NfcFn& nfc = {});

std::string to_lower_hex(const std::vector<unsigned char>& digest);

}  // namespace kirivers
