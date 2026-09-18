#include <kirivers/client.hpp>
#include <kirivers/hosted.hpp>

namespace kirivers {

void apply_hosted_defaults(Config& cfg) {
  if (!cfg.transport) cfg.transport = std::make_shared<CurlTransport>();
  if (!cfg.hasher) cfg.hasher = std::make_shared<OpenSslHasher>();
  if (!cfg.file_store) cfg.file_store = std::make_shared<FilesystemStore>(".");
  if (!cfg.archive_unpacker) cfg.archive_unpacker = std::make_shared<LibzipUnpacker>();
  if (!cfg.signature_verifier) {
    cfg.signature_verifier = std::make_shared<OpenSslSignatureVerifier>();
  }
}

}  // namespace kirivers
