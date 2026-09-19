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

// Case-insensitive hex compare (server SHA-256 is lowercase; hashers may differ).
bool equal_hex(std::string_view a, std::string_view b);

}  // namespace kirivers
