#pragma once

#include <kirivers/http.hpp>

#include <string>

namespace kirivers {

enum class DeltaMagic { KvDiffHp1, Hdiff13, Bsdiff40, Vcdiff, Unknown };

DeltaMagic detect_delta_magic(const Bytes& delta);
bool magic_matches_algo(DeltaMagic magic, const std::string& algo);

}  // namespace kirivers
