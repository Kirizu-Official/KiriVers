package official.kirizu.kirivers.client.adapter;

import java.io.IOException;
import java.util.List;

/**
 * Binary delta apply. Official SDKs other than Go ship no default implementation.
 * Inject one only for algorithms you can actually apply; unknown magic must fail
 * (never cross-decode).
 */
public interface Patcher {

  /** Wire names: {@code hdiffpatch}, {@code bsdiff}, {@code xdelta3}. */
  List<String> supportedAlgos();

  byte[] apply(byte[] source, byte[] delta) throws IOException;
}
