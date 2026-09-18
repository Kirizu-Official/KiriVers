package official.kirizu.kirivers.client.adapter;

import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.KeyFactory;
import java.security.PublicKey;
import java.security.Signature;
import java.security.spec.X509EncodedKeySpec;
import java.util.Base64;
import java.util.Locale;

/** Ed25519 ({@code EdDSA}) and RSA-SHA256 using the JDK 15+ providers. */
public final class JdkSignatureVerifier implements SignatureVerifier {

  @Override
  public void verify(String algo, String publicKeyPem, byte[] payload, String signatureBase64)
      throws SignatureException {
    if (algo == null) {
      throw new SignatureException("signature algorithm is required");
    }
    byte[] sig;
    try {
      sig = Base64.getDecoder().decode(signatureBase64);
    } catch (IllegalArgumentException e) {
      throw new SignatureException("invalid signature encoding", e);
    }
    String normalized = algo.trim().toLowerCase(Locale.ROOT);
    try {
      if (SigningKey.ED25519.equals(normalized) || "eddsa".equals(normalized)) {
        PublicKey key = parsePem(publicKeyPem, "Ed25519");
        Signature verifier = Signature.getInstance("Ed25519");
        verifier.initVerify(key);
        verifier.update(payload);
        if (!verifier.verify(sig)) {
          throw new SignatureException("ed25519 signature mismatch");
        }
        return;
      }
      if (SigningKey.RSA_SHA256.equals(normalized) || "rsasha256".equals(normalized)) {
        PublicKey key = parsePem(publicKeyPem, "RSA");
        Signature verifier = Signature.getInstance("SHA256withRSA");
        verifier.initVerify(key);
        verifier.update(payload);
        if (!verifier.verify(sig)) {
          throw new SignatureException("rsa-sha256 signature mismatch");
        }
        return;
      }
      throw new SignatureException("unsupported signature algorithm: " + algo);
    } catch (SignatureException e) {
      throw e;
    } catch (GeneralSecurityException e) {
      throw new SignatureException("signature verify failed", e);
    }
  }

  public static byte[] buildCheckPayload(
      String versionInteger,
      String versionSemver,
      String rootHash,
      String packageUrl,
      String size,
      String sha256Hex) {
    String iv = versionInteger == null ? "" : versionInteger;
    String sv = versionSemver == null ? "" : versionSemver;
    String rh = rootHash == null ? "" : rootHash;
    String pu = packageUrl == null ? "" : packageUrl;
    String sz = size == null ? "" : size;
    String sh = sha256Hex == null ? "" : sha256Hex;
    return String.join("\n", iv, sv, rh, pu, sz, sh).getBytes(StandardCharsets.UTF_8);
  }

  public static String decimalOrEmpty(Number n) {
    return n == null ? "" : Long.toString(n.longValue());
  }

  private static PublicKey parsePem(String pem, String keyFactoryAlgo) throws GeneralSecurityException {
    if (pem == null || pem.isBlank()) {
      throw new GeneralSecurityException("public key PEM is required");
    }
    String b64 =
        pem.replaceAll("-----BEGIN [^-]+-----", "")
            .replaceAll("-----END [^-]+-----", "")
            .replaceAll("\\s+", "");
    byte[] der = Base64.getDecoder().decode(b64);
    KeyFactory kf = KeyFactory.getInstance(keyFactoryAlgo);
    return kf.generatePublic(new X509EncodedKeySpec(der));
  }
}
