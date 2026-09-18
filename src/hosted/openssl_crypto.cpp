#include <kirivers/hosted.hpp>
#include <kirivers/error.hpp>
#include <kirivers/path.hpp>

#include <openssl/bio.h>
#include <openssl/evp.h>
#include <openssl/pem.h>
#include <fstream>
#include <memory>
#include <stdexcept>
#include <vector>

namespace kirivers {

namespace {

struct BioFree {
  void operator()(BIO* b) const { if (b) BIO_free_all(b); }
};
struct PkeyFree {
  void operator()(EVP_PKEY* p) const { if (p) EVP_PKEY_free(p); }
};
struct MdCtxFree {
  void operator()(EVP_MD_CTX* c) const { if (c) EVP_MD_CTX_free(c); }
};

std::string digest_hex(const EVP_MD* md, const unsigned char* data, size_t n) {
  unsigned char out[EVP_MAX_MD_SIZE];
  unsigned int len = 0;
  if (EVP_Digest(data, n, out, &len, md, nullptr) != 1) {
    throw std::runtime_error("EVP_Digest failed");
  }
  return to_lower_hex(std::vector<unsigned char>(out, out + len));
}

std::string digest_file(const EVP_MD* md, const std::string& path) {
  std::ifstream in(path, std::ios::binary);
  if (!in) throw std::runtime_error("cannot open file for hashing");
  std::unique_ptr<EVP_MD_CTX, MdCtxFree> ctx(EVP_MD_CTX_new());
  if (!ctx || EVP_DigestInit_ex(ctx.get(), md, nullptr) != 1) {
    throw std::runtime_error("EVP_DigestInit failed");
  }
  char buf[8192];
  while (in) {
    in.read(buf, sizeof(buf));
    auto n = in.gcount();
    if (n > 0 && EVP_DigestUpdate(ctx.get(), buf, static_cast<size_t>(n)) != 1) {
      throw std::runtime_error("EVP_DigestUpdate failed");
    }
  }
  unsigned char out[EVP_MAX_MD_SIZE];
  unsigned int len = 0;
  if (EVP_DigestFinal_ex(ctx.get(), out, &len) != 1) {
    throw std::runtime_error("EVP_DigestFinal failed");
  }
  return to_lower_hex(std::vector<unsigned char>(out, out + len));
}

Bytes b64_decode(const std::string& s) {
  std::unique_ptr<BIO, BioFree> b64(BIO_new(BIO_f_base64()));
  BIO_set_flags(b64.get(), BIO_FLAGS_BASE64_NO_NL);
  BIO* mem = BIO_new_mem_buf(s.data(), static_cast<int>(s.size()));
  BIO_push(b64.get(), mem);
  Bytes out(s.size());
  const int n = BIO_read(b64.get(), out.data(), static_cast<int>(out.size()));
  if (n < 0) throw ApiError(0, "BAD_SIGNATURE", "invalid base64 signature");
  out.resize(static_cast<size_t>(n));
  return out;
}

}  // namespace

std::string OpenSslHasher::sha256_hex(const Bytes& data) const {
  return digest_hex(EVP_sha256(), data.data(), data.size());
}

std::string OpenSslHasher::sha256_file(const std::string& path) const {
  return digest_file(EVP_sha256(), path);
}

std::string OpenSslHasher::md5_hex(const Bytes& data) const {
  return digest_hex(EVP_md5(), data.data(), data.size());
}

void OpenSslSignatureVerifier::verify(const std::string& algo,
                                      const std::string& public_key_pem,
                                      const std::string& payload,
                                      const std::string& signature_b64) const {
  std::unique_ptr<BIO, BioFree> bio(
      BIO_new_mem_buf(public_key_pem.data(), static_cast<int>(public_key_pem.size())));
  if (!bio) throw ApiError(0, "BAD_PUBLIC_KEY", "cannot wrap public key PEM");
  EVP_PKEY* raw = PEM_read_bio_PUBKEY(bio.get(), nullptr, nullptr, nullptr);
  if (!raw) throw ApiError(0, "BAD_PUBLIC_KEY", "invalid public key pem");
  std::unique_ptr<EVP_PKEY, PkeyFree> pkey(raw);

  const Bytes sig = b64_decode(signature_b64);
  std::unique_ptr<EVP_MD_CTX, MdCtxFree> ctx(EVP_MD_CTX_new());
  if (!ctx) throw std::runtime_error("EVP_MD_CTX_new failed");

  const EVP_MD* md = nullptr;
  if (algo == "rsa-sha256") md = EVP_sha256();
  else if (algo != "ed25519") {
    throw ApiError(0, "UNSUPPORTED_ALGO", "unsupported signature algorithm");
  }

  if (EVP_DigestVerifyInit(ctx.get(), nullptr, md, nullptr, pkey.get()) != 1) {
    throw ApiError(0, "BAD_PUBLIC_KEY", "DigestVerifyInit failed");
  }
  const int rc = EVP_DigestVerify(
      ctx.get(), sig.data(), sig.size(),
      reinterpret_cast<const unsigned char*>(payload.data()), payload.size());
  if (rc != 1) throw ApiError(0, "BAD_SIGNATURE", "signature mismatch");
}

}  // namespace kirivers
