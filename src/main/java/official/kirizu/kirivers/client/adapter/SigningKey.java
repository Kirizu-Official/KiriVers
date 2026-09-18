package official.kirizu.kirivers.client.adapter;

import java.util.Locale;
import java.util.Objects;

/** Project public key used to verify check/integrity/diff {@code signature}. */
public final class SigningKey {

  public static final String ED25519 = "ed25519";
  public static final String RSA_SHA256 = "rsa-sha256";

  private final String algo;
  private final String publicKeyPem;

  public SigningKey(String algo, String publicKeyPem) {
    this.algo = Objects.requireNonNull(algo, "algo").trim().toLowerCase(Locale.ROOT);
    this.publicKeyPem = Objects.requireNonNull(publicKeyPem, "publicKeyPem");
  }

  public String algo() {
    return algo;
  }

  public String publicKeyPem() {
    return publicKeyPem;
  }
}
