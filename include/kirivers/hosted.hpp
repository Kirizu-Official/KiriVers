#pragma once

#include "adapters.hpp"

#include <memory>
#include <string>

namespace kirivers {

#if defined(KIRIVERS_HOSTED)

class CurlTransport : public Transport {
 public:
  CurlTransport();
  ~CurlTransport() override;
  CurlTransport(const CurlTransport&) = delete;
  CurlTransport& operator=(const CurlTransport&) = delete;
  HttpResponse execute(const HttpRequest& req) override;

 private:
  struct Impl;
  std::unique_ptr<Impl> impl_;
};

class OpenSslHasher : public Hasher {
 public:
  std::string sha256_hex(const Bytes& data) const override;
  std::string sha256_file(const std::string& path) const override;
  std::string md5_hex(const Bytes& data) const override;
};

class OpenSslSignatureVerifier : public SignatureVerifier {
 public:
  void verify(const std::string& algo, const std::string& public_key_pem,
              const std::string& payload,
              const std::string& signature_b64) const override;
};

class FilesystemStore : public FileStore {
 public:
  explicit FilesystemStore(std::string root);
  Bytes read_file(const std::string& path) const override;
  void write_file(const std::string& path, const Bytes& data) override;
  bool exists(const std::string& path) const override;
  std::vector<std::string> list_files(const std::string& dir) const override;
  std::string normalize_path(std::string_view raw) const override;
  bool can_write_individual_files() const override { return true; }
  const std::string& root() const { return root_; }

 private:
  std::string root_;
};

class LibzipUnpacker : public ArchiveUnpacker {
 public:
  void unpack_zip(const Bytes& zip, FileStore& dest, const std::string& dest_root,
                  const std::vector<std::pair<std::string, std::string>>&
                      sha256_to_relpath) override;
};

#if defined(_WIN32)
class MoveFileExReplacer : public Replacer {
 public:
  void replace(const std::string& staged_path,
               const std::string& dest_path) override;
};
#endif

void apply_hosted_defaults(struct Config& cfg);

std::string utf8proc_nfc(std::string_view s);

#endif  // KIRIVERS_HOSTED

}  // namespace kirivers
