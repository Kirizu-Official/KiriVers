package official.kirizu.kirivers.client.adapter;

import java.util.List;

/**
 * Verifies {@code signature} over {@code BuildCheckPayload}:
 * {@code version_integer \\n version_semver \\n root_hash \\n package_url \\n size \\n sha256}.
 */
public interface SignatureVerifier {

  void verify(String algo, String publicKeyPem, byte[] payload, String signatureBase64)
      throws SignatureException;

  default void verifyAny(List<SigningKey> keys, byte[] payload, String signatureBase64)
      throws SignatureException {
    if (keys == null || keys.isEmpty()) {
      throw new SignatureException("no public keys configured");
    }
    SignatureException last = null;
    for (SigningKey key : keys) {
      try {
        verify(key.algo(), key.publicKeyPem(), payload, signatureBase64);
        return;
      } catch (SignatureException e) {
        last = e;
      }
    }
    throw last == null ? new SignatureException("signature mismatch") : last;
  }

  final class SignatureException extends Exception {
    public SignatureException(String message) {
      super(message);
    }

    public SignatureException(String message, Throwable cause) {
      super(message, cause);
    }
  }
}
