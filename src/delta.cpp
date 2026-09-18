#include "delta.hpp"

#include <cstring>

namespace kirivers {

DeltaMagic detect_delta_magic(const Bytes& delta) {
  auto starts = [&](const char* lit) {
    const size_t n = std::strlen(lit);
    if (delta.size() < n) return false;
    return std::memcmp(delta.data(), lit, n) == 0;
  };
  if (starts("KVDIFFHP1\n")) return DeltaMagic::KvDiffHp1;
  if (starts("HDIFF13&")) return DeltaMagic::Hdiff13;
  if (starts("BSDIFF40")) return DeltaMagic::Bsdiff40;
  if (delta.size() >= 3 && static_cast<unsigned char>(delta[0]) == 0xD6 &&
      static_cast<unsigned char>(delta[1]) == 0xC3 &&
      static_cast<unsigned char>(delta[2]) == 0xC4) {
    return DeltaMagic::Vcdiff;
  }
  return DeltaMagic::Unknown;
}

bool magic_matches_algo(DeltaMagic magic, const std::string& algo) {
  if (algo == "hdiffpatch") {
    return magic == DeltaMagic::Hdiff13 || magic == DeltaMagic::KvDiffHp1;
  }
  if (algo == "bsdiff") return magic == DeltaMagic::Bsdiff40;
  if (algo == "xdelta3") return magic == DeltaMagic::Vcdiff;
  return false;
}

}  // namespace kirivers
