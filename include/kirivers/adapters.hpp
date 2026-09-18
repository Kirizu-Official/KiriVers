#pragma once

#include "http.hpp"
#include "path.hpp"

#include <functional>
#include <memory>
#include <stdexcept>
#include <string>
#include <string_view>
#include <vector>

namespace kirivers {

class Transport {
 public:
  virtual ~Transport() = default;
  virtual HttpResponse execute(const HttpRequest& req) = 0;
};

class FunctionTransport : public Transport {
 public:
  explicit FunctionTransport(std::function<HttpResponse(const HttpRequest&)> fn)
      : fn_(std::move(fn)) {}
  HttpResponse execute(const HttpRequest& req) override { return fn_(req); }

 private:
  std::function<HttpResponse(const HttpRequest&)> fn_;
};

class Hasher {
 public:
  virtual ~Hasher() = default;
  virtual std::string sha256_hex(const Bytes& data) const = 0;
  virtual std::string sha256_file(const std::string& path) const = 0;
  virtual std::string md5_hex(const Bytes& data) const = 0;
};

class FunctionHasher : public Hasher {
 public:
  using HashFn = std::function<std::string(const Bytes&)>;
  FunctionHasher(HashFn sha, HashFn md5 = {})
      : sha_(std::move(sha)), md5_(std::move(md5)) {}
  std::string sha256_hex(const Bytes& data) const override { return sha_(data); }
  std::string sha256_file(const std::string& path) const override;
  std::string md5_hex(const Bytes& data) const override {
    return md5_ ? md5_(data) : std::string{};
  }

 private:
  HashFn sha_;
  HashFn md5_;
};

class FileStore {
 public:
  virtual ~FileStore() = default;
  virtual Bytes read_file(const std::string& path) const = 0;
  virtual void write_file(const std::string& path, const Bytes& data) = 0;
  virtual bool exists(const std::string& path) const = 0;
  virtual std::vector<std::string> list_files(const std::string& dir) const = 0;
  virtual std::string normalize_path(std::string_view raw) const {
    auto n = normalize_rel_path(raw);
    if (!n) throw std::invalid_argument("invalid relative path");
    return *n;
  }
  virtual bool can_write_individual_files() const { return true; }
};

class Patcher {
 public:
  virtual ~Patcher() = default;
  // Names the server understands: hdiffpatch, bsdiff, xdelta3.
  virtual std::vector<std::string> supported_algos() const = 0;
  virtual Bytes apply(const Bytes& old_bytes, const Bytes& delta,
                      const std::string& algo) = 0;
};

class FunctionPatcher : public Patcher {
 public:
  FunctionPatcher(std::vector<std::string> algos,
                  std::function<Bytes(const Bytes&, const Bytes&, const std::string&)> fn)
      : algos_(std::move(algos)), fn_(std::move(fn)) {}
  std::vector<std::string> supported_algos() const override { return algos_; }
  Bytes apply(const Bytes& old_bytes, const Bytes& delta,
              const std::string& algo) override {
    return fn_(old_bytes, delta, algo);
  }

 private:
  std::vector<std::string> algos_;
  std::function<Bytes(const Bytes&, const Bytes&, const std::string&)> fn_;
};

class Replacer {
 public:
  virtual ~Replacer() = default;
  virtual void replace(const std::string& staged_path,
                       const std::string& dest_path) = 0;
};

class ArchiveUnpacker {
 public:
  virtual ~ArchiveUnpacker() = default;
  // Native patch zip members are content SHA-256 hex; map them with files[].path.
  virtual void unpack_zip(const Bytes& zip, FileStore& dest,
                          const std::string& dest_root,
                          const std::vector<std::pair<std::string, std::string>>&
                              sha256_to_relpath) = 0;
};

class SignatureVerifier {
 public:
  virtual ~SignatureVerifier() = default;
  // algo: "ed25519" or "rsa-sha256". signature_b64 is standard base64.
  virtual void verify(const std::string& algo, const std::string& public_key_pem,
                      const std::string& payload,
                      const std::string& signature_b64) const = 0;
};

// Check-payload bytes: integer \n semver \n root_hash \n package_url \n size \n sha256
std::string build_check_payload(const std::string& version_integer,
                                const std::string& version_semver,
                                const std::string& root_hash,
                                const std::string& package_url,
                                const std::string& size,
                                const std::string& sha256_hex);

}  // namespace kirivers
