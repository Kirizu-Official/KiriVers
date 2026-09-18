package official.kirizu.kirivers.client;

import java.text.Normalizer;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * Integrity / pack path identity: {@code \} → {@code /}, Unicode NFC, reject {@code ..}.
 * Matches the server fileset rules used for root-hash and pack identity.
 */
public final class PathUtil {

  private PathUtil() {}

  public static String normalize(String path) {
    if (path == null) {
      throw new IllegalArgumentException("path is required");
    }
    String nfc = Normalizer.normalize(path.replace('\\', '/'), Normalizer.Form.NFC);
    List<String> parts = new ArrayList<>();
    for (String raw : nfc.split("/")) {
      if (raw.isEmpty() || ".".equals(raw)) {
        continue;
      }
      if ("..".equals(raw)) {
        throw new IllegalArgumentException("path must not contain '..'");
      }
      parts.add(raw);
    }
    return String.join("/", parts);
  }

  public static boolean isKeepIfExists(String policy) {
    return policy != null && "KEEP_IF_EXISTS".equalsIgnoreCase(policy.trim());
  }

  public static boolean hashesEqual(String a, String b) {
    if (a == null || b == null) {
      return false;
    }
    return a.trim().toLowerCase(Locale.ROOT).equals(b.trim().toLowerCase(Locale.ROOT));
  }
}
