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

  /** Wire names used in {@code accepted_delta_algos} / {@code Patcher.supportedAlgos()}. */
  public static final String ALGO_HDIFFPATCH = "hdiffpatch";
  public static final String ALGO_BSDIFF = "bsdiff";
  public static final String ALGO_XDELTA3 = "xdelta3";

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
    wireAlgo(delta);
  }

  /**
   * Map container magic to the check/diff wire algorithm. Unknown prefixes must
   * not be decoded as another engine.
   */
  public static String wireAlgo(byte[] delta) throws IOException {
    String kind = detect(delta);
    if (KVDIFFHP1.equals(kind) || HDIFF13.equals(kind)) {
      return ALGO_HDIFFPATCH;
    }
    if (BSDIFF40.equals(kind)) {
      return ALGO_BSDIFF;
    }
    if (VCDIFF.equals(kind)) {
      return ALGO_XDELTA3;
    }
    throw new IOException("unknown delta magic; refusing to cross-decode");
  }

  /**
   * Refuse apply unless the live Patcher advertised the wire algo for this magic.
   * Prevents a bsdiff-only Patcher from seeing HDIFF13/VCDIFF bytes.
   */
  public static void requireSupported(Iterable<String> supportedAlgos, byte[] delta) throws IOException {
    String algo = wireAlgo(delta);
    if (supportedAlgos != null) {
      for (String raw : supportedAlgos) {
        if (raw != null && algo.equalsIgnoreCase(raw.trim())) {
          return;
        }
      }
    }
    throw new IOException("patcher does not support " + algo + "; refusing to cross-decode");
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
