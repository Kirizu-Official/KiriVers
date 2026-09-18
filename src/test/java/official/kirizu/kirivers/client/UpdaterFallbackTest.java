package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.DeltaMagic;
import official.kirizu.kirivers.client.adapter.Hasher;
import official.kirizu.kirivers.client.adapter.MessageDigestHasher;
import official.kirizu.kirivers.client.adapter.Patcher;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

class UpdaterFallbackTest {

  @Test
  void unknownDeltaMagicFallsBackToFullPackage(@TempDir Path tmp) throws Exception {
    Hasher hasher = new MessageDigestHasher();
    byte[] full = "FULL-PACKAGE-BYTES".getBytes(StandardCharsets.UTF_8);
    String fullSha = hasher.sha256(full);
    RecordingTransport transport = new RecordingTransport();
    AtomicInteger downloads = new AtomicInteger();
    transport.handler(
        req -> {
          if (req.url().contains("/update/check")) {
            return RecordingTransport.json(
                200,
                "{\"has_update\":true,\"is_mandatory\":false,\"is_downgrade\":false,\"reason\":\"normal\","
                    + "\"compare_engine\":\"semver\",\"version_integer\":null,\"version_semver\":\"1.1.0\","
                    + "\"target_channel\":\"stable\",\"target_hw_rev\":null,\"package_type\":\"single_file\","
                    + "\"root_hash\":\"\",\"package_url\":\"/api/v1/projects/p/packages/"
                    + fullSha
                    + "\",\"file_name\":\"app.bin\",\"size\":"
                    + full.length
                    + ",\"sha256\":\""
                    + fullSha
                    + "\",\"delta_available\":true,\"delta_algo\":\"bsdiff\"}");
          }
          if (req.url().contains("/update/diff")) {
            return RecordingTransport.json(
                200,
                "{\"diff_mode\":\"binary_delta\",\"root_hash\":\"\",\"version_integer\":null,"
                    + "\"version_semver\":\"1.1.0\",\"channel\":\"stable\",\"compare_engine\":\"semver\","
                    + "\"package_url\":\"/api/v1/projects/p/packages/delta\",\"sha256\":\"00\"}");
          }
          if (req.url().contains("/packages/delta")) {
            return RecordingTransport.bytes(200, "NOT-A-DELTA".getBytes(StandardCharsets.US_ASCII));
          }
          if (req.url().contains("/packages/") && req.method().equals("GET")) {
            downloads.incrementAndGet();
            return RecordingTransport.bytes(200, full);
          }
          if (req.url().contains("/telemetry/report")) {
            return RecordingTransport.json(202, "{\"status\":\"accepted\"}");
          }
          return RecordingTransport.json(404, "{\"error\":{\"code\":\"NOT_FOUND\",\"message\":\"x\"}}");
        });
    Patcher patcher =
        new Patcher() {
          @Override
          public List<String> supportedAlgos() {
            return List.of("bsdiff");
          }

          @Override
          public byte[] apply(byte[] source, byte[] delta) {
            throw new AssertionError("must not apply unknown magic: " + DeltaMagic.detect(delta));
          }
        };
    Client client =
        new Client(
            Config.builder()
                .baseUrl("http://127.0.0.1:8080")
                .projectRef("p")
                .transport(transport)
                .patcher(patcher)
                .build());
    Updater updater = new Updater(client);
    UpdateRequest req = new UpdateRequest();
    req.currentVersion = "1.0.0";
    req.os = "windows";
    req.arch = "x86_64";
    req.channel = "stable";
    req.localFile = "OLD".getBytes(StandardCharsets.UTF_8);
    req.stageDir = tmp;
    UpdateResult result = updater.update(req);
    assertEquals(UpdateResult.Status.STAGED, result.status());
    assertEquals("full_package", result.diffMode());
    assertNotNull(result.stagedPath());
    assertEquals(fullSha, result.sha256());
  }
}
