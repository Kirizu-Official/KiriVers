package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.DeltaMagic;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class DeltaMagicTest {

  @Test
  void splitsKnownPrefixes() throws Exception {
    assertEquals(DeltaMagic.KVDIFFHP1, DeltaMagic.detect("KVDIFFHP1\nrest".getBytes(StandardCharsets.US_ASCII)));
    assertEquals(DeltaMagic.HDIFF13, DeltaMagic.detect("HDIFF13&rest".getBytes(StandardCharsets.US_ASCII)));
    assertEquals(DeltaMagic.BSDIFF40, DeltaMagic.detect("BSDIFF40rest".getBytes(StandardCharsets.US_ASCII)));
    assertEquals(DeltaMagic.VCDIFF, DeltaMagic.detect(new byte[] {(byte) 0xD6, (byte) 0xC3, (byte) 0xC4, 0x00}));
    assertEquals(DeltaMagic.UNKNOWN, DeltaMagic.detect("nope".getBytes(StandardCharsets.US_ASCII)));
    assertTrue(DeltaMagic.isUnknown(new byte[] {1, 2, 3}));
    assertEquals(DeltaMagic.ALGO_HDIFFPATCH, DeltaMagic.wireAlgo("KVDIFFHP1\nrest".getBytes(StandardCharsets.US_ASCII)));
    assertEquals(DeltaMagic.ALGO_HDIFFPATCH, DeltaMagic.wireAlgo("HDIFF13&rest".getBytes(StandardCharsets.US_ASCII)));
    assertEquals(DeltaMagic.ALGO_BSDIFF, DeltaMagic.wireAlgo("BSDIFF40rest".getBytes(StandardCharsets.US_ASCII)));
    assertEquals(DeltaMagic.ALGO_XDELTA3, DeltaMagic.wireAlgo(new byte[] {(byte) 0xD6, (byte) 0xC3, (byte) 0xC4}));
  }
}
