package official.kirizu.kirivers.client.adapter;

import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.HexFormat;

/** Default {@link Hasher} using {@link MessageDigest}. */
public final class MessageDigestHasher implements Hasher {

  private static final HexFormat HEX = HexFormat.of();

  @Override
  public String sha256(byte[] data) {
    return digest("SHA-256", data);
  }

  @Override
  public String sha256(InputStream in) throws IOException {
    return digest("SHA-256", in);
  }

  @Override
  public String sha256(Path file) throws IOException {
    try (InputStream in = Files.newInputStream(file)) {
      return sha256(in);
    }
  }

  @Override
  public String md5(byte[] data) {
    return digest("MD5", data);
  }

  @Override
  public String md5(InputStream in) throws IOException {
    return digest("MD5", in);
  }

  @Override
  public String md5(Path file) throws IOException {
    try (InputStream in = Files.newInputStream(file)) {
      return md5(in);
    }
  }

  private static String digest(String algo, byte[] data) {
    try {
      return HEX.formatHex(MessageDigest.getInstance(algo).digest(data == null ? new byte[0] : data));
    } catch (NoSuchAlgorithmException e) {
      throw new IllegalStateException(algo + " is required", e);
    }
  }

  private static String digest(String algo, InputStream in) throws IOException {
    try {
      MessageDigest md = MessageDigest.getInstance(algo);
      byte[] buf = new byte[8192];
      int n;
      while ((n = in.read(buf)) >= 0) {
        if (n > 0) {
          md.update(buf, 0, n);
        }
      }
      return HEX.formatHex(md.digest());
    } catch (NoSuchAlgorithmException e) {
      throw new IllegalStateException(algo + " is required", e);
    }
  }
}
