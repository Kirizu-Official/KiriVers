package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.JdkSignatureVerifier;
import official.kirizu.kirivers.client.adapter.SigningKey;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.Signature;
import java.util.Base64;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class SignatureVerifierTest {

  @Test
  void payloadJoinsWithNewlinesAndEmptyPlaceholders() {
    byte[] got = JdkSignatureVerifier.buildCheckPayload("102", "1.2.3", "roothash", "/pkg", "123456", "abcd");
    assertArrayEquals("102\n1.2.3\nroothash\n/pkg\n123456\nabcd".getBytes(StandardCharsets.UTF_8), got);
    byte[] empty = JdkSignatureVerifier.buildCheckPayload("", "1.2.3", "", "/pkg", "", "abcd");
    assertArrayEquals("\n1.2.3\n\n/pkg\n\nabcd".getBytes(StandardCharsets.UTF_8), empty);
  }

  @Test
  void verifiesEd25519AndRsa() throws Exception {
    JdkSignatureVerifier v = new JdkSignatureVerifier();
    byte[] payload = JdkSignatureVerifier.buildCheckPayload("102", "1.2.3", "root", "/u", "1", "aa");

    KeyPairGenerator ed = KeyPairGenerator.getInstance("Ed25519");
    KeyPair edPair = ed.generateKeyPair();
    Signature edSig = Signature.getInstance("Ed25519");
    edSig.initSign(edPair.getPrivate());
    edSig.update(payload);
    String edB64 = Base64.getEncoder().encodeToString(edSig.sign());
    String edPem = pem("PUBLIC KEY", edPair.getPublic().getEncoded());
    v.verify(SigningKey.ED25519, edPem, payload, edB64);
    assertThrows(
        JdkSignatureVerifier.SignatureException.class,
        () -> v.verify(SigningKey.ED25519, edPem, "tamper".getBytes(StandardCharsets.UTF_8), edB64));

    KeyPairGenerator rsa = KeyPairGenerator.getInstance("RSA");
    rsa.initialize(2048);
    KeyPair rsaPair = rsa.generateKeyPair();
    Signature rsaSig = Signature.getInstance("SHA256withRSA");
    rsaSig.initSign(rsaPair.getPrivate());
    rsaSig.update(payload);
    String rsaB64 = Base64.getEncoder().encodeToString(rsaSig.sign());
    String rsaPem = pem("PUBLIC KEY", rsaPair.getPublic().getEncoded());
    v.verify(SigningKey.RSA_SHA256, rsaPem, payload, rsaB64);
  }

  private static String pem(String type, byte[] der) {
    return "-----BEGIN " + type + "-----\n" + Base64.getMimeEncoder().encodeToString(der) + "\n-----END " + type + "-----\n";
  }
}
