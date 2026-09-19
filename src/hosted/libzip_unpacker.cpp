#include <kirivers/hosted.hpp>
#include <kirivers/error.hpp>

#include <zip.h>
#include <cstring>
#include <stdexcept>
#include <vector>

namespace kirivers {

void LibzipUnpacker::unpack_zip(
    const Bytes& zip, FileStore& dest, const std::string& dest_root,
    const std::vector<std::pair<std::string, std::string>>& sha256_to_relpath) {
  zip_error_t err;
  zip_error_init(&err);
  zip_source_t* src = zip_source_buffer_create(zip.data(), zip.size(), 0, &err);
  if (!src) {
    std::string m = zip_error_strerror(&err);
    zip_error_fini(&err);
    throw std::runtime_error(m);
  }
  zip_t* za = zip_open_from_source(src, 0, &err);
  if (!za) {
    zip_source_free(src);
    std::string m = zip_error_strerror(&err);
    zip_error_fini(&err);
    throw std::runtime_error(m);
  }
  zip_error_fini(&err);

  try {
    for (const auto& kv : sha256_to_relpath) {
      const std::string& hex = kv.first;
      const std::string& rel = kv.second;
      zip_stat_t st;
      zip_stat_init(&st);
      if (zip_stat(za, hex.c_str(), 0, &st) != 0) {
        throw std::runtime_error("zip member missing for " + hex);
      }
      if ((st.valid & ZIP_STAT_SIZE) == 0) {
        throw std::runtime_error("zip member size unknown for " + hex);
      }
      zip_file_t* zf = zip_fopen(za, hex.c_str(), 0);
      if (!zf) throw std::runtime_error("zip_fopen failed");
      Bytes buf(static_cast<size_t>(st.size));
      const zip_int64_t n = zip_fread(zf, buf.data(), buf.size());
      zip_fclose(zf);
      if (n < 0 || static_cast<size_t>(n) != buf.size()) {
        throw std::runtime_error("zip_fread short read");
      }
      const std::string out_path = dest_root.empty() ? rel : dest_root + "/" + rel;
      dest.write_file(out_path, buf);
    }
  } catch (...) {
    zip_close(za);
    throw;
  }
  zip_close(za);
}

}  // namespace kirivers
