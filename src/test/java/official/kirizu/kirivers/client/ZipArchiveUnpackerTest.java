package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.ZipArchiveUnpacker;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Map;
import java.util.zip.ZipEntry;
import java.util.zip.ZipOutputStream;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ZipArchiveUnpackerTest {

  @Test
  void mapsHashNamedMembersToIntegrityPaths(@TempDir Path dest) throws Exception {
    String sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    byte[] zip = zip(sha, "payload".getBytes(StandardCharsets.UTF_8));
    new ZipArchiveUnpacker().unpack(zip, Map.of(sha, "bin/app"), dest);
    Path out = dest.resolve("bin").resolve("app");
    assertTrue(Files.isRegularFile(out));
    assertEquals("payload", Files.readString(out));
  }

  @Test
  void rejectsMappedPathWithDotDot(@TempDir Path dest) throws Exception {
    String sha = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
    byte[] zip = zip(sha, "nope".getBytes(StandardCharsets.UTF_8));
    assertThrows(
        IllegalArgumentException.class,
        () -> new ZipArchiveUnpacker().unpack(zip, Map.of(sha, "../outside"), dest));
  }

  private static byte[] zip(String name, byte[] bytes) throws IOException {
    ByteArrayOutputStream bos = new ByteArrayOutputStream();
    try (ZipOutputStream zos = new ZipOutputStream(bos)) {
      zos.putNextEntry(new ZipEntry(name));
      zos.write(bytes);
      zos.closeEntry();
    }
    return bos.toByteArray();
  }
}