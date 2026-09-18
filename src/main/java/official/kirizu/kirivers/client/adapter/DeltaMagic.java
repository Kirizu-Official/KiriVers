package official.kirizu.kirivers.client.adapter;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;

/**
 * Delta container magic. Unknown prefixes must not be decoded as another engine.
 *
 * <ul>
 *   <li>{@code KVDIFFHP1\\n} — KiriVers pure-Go hdiffpatch fallback</li>
 *   <li>{@code HDIFF13&} — official HDiffPatch</li>
 *   <li>{@code BSDIFF40} — bsdiff4</li>
 *   <li>{@code D6 C3 C4} — VCDIFF / xdelta3</li>
 * </ul>
 */
public final class DeltaMagic {

  public static final String KVDIFFHP1 = "KVDIFFHP1";
  public static final String HDIFF13 = "HDIFF13";
  public static final String BSDIFF40 = "BSDIFF40";
  public static final String VCDIFF = "VCDIFF";
  public static final String UNKNOWN = "UNKNOWN";

  private static final byte[] MAGIC_KV = "KVDIFFHP1\n".getBytes(StandardCharsets.US_ASCII);
  private static final byte[] MAGIC_HDIFF = "HDIFF13&".getBytes(StandardCharsets.US_ASCII);
  private static final byte[] MAGIC_BSDIFF = "BSDIFF40".getBytes(StandardCharsets.US_ASCII);
  private static final byte[] MAGIC_VCDIFF = new byte[] {(byte) 0xD6, (byte) 0xC3, (byte) 0xC4};

  private DeltaMagic() {}

  public static String detect(byte[] delta) {
    if (startsWith(delta, MAGIC_KV)) {
      return KVDIFFHP1;
    }
    if (startsWith(delta, MAGIC_HDIFF)) {
      return HDIFF13;
    }
    if (startsWith(delta, MAGIC_BSDIFF)) {
      return BSDIFF40;
    }
    if (startsWith(delta, MAGIC_VCDIFF)) {
      return VCDIFF;
    }
    return UNKNOWN;
  }

  public static void requireKnown(byte[] delta) throws IOException {
    String kind = detect(delta);
    if (UNKNOWN.equals(kind)) {
      throw new IOException("unknown delta magic; refusing to cross-decode");
    }
  }

  public static boolean isUnknown(byte[] delta) {
    return UNKNOWN.equals(detect(delta));
  }

  private static boolean startsWith(byte[] data, byte[] prefix) {
    if (data == null || data.length < prefix.length) {
      return false;
    }
    return Arrays.equals(Arrays.copyOfRange(data, 0, prefix.length), prefix);
  }
}
