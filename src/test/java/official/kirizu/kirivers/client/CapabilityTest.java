package official.kirizu.kirivers.client;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import official.kirizu.kirivers.client.adapter.Patcher;
import official.kirizu.kirivers.client.model.UpdateCheckRequest;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CapabilityTest {

  @Test
  void defaultCheckOmitsBinaryDelta() throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Client client =
        new Client(Config.builder().baseUrl("http://127.0.0.1:8080").projectRef("sdk-fixture").transport(transport).build());
    UpdateCheckRequest req = new UpdateCheckRequest();
    req.currentVersion = "1.0.0";
    req.os = "windows";
    req.arch = "x86_64";
    client.check(req);
    JsonNode body = Json.MAPPER.readTree(new String(transport.last().body(), StandardCharsets.UTF_8));
    List<String> caps = Json.MAPPER.convertValue(body.get("capabilities"), new TypeReference<List<String>>() {});
    assertTrue(caps.contains("full_package"));
    assertTrue(caps.contains("patch_package"));
    assertTrue(caps.contains("file_list"));
    assertFalse(caps.contains("binary_delta"));
    assertFalse(body.has("accepted_delta_algos"));
  }

  @Test
  void injectedPatcherAdvertisesAlgos() throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Patcher patcher =
        new Patcher() {
          @Override
          public List<String> supportedAlgos() {
            return List.of("bsdiff", "xdelta3", "hdiffpatch");
          }

          @Override
          public byte[] apply(byte[] source, byte[] delta) throws IOException {
            throw new IOException("unused");
          }
        };
    Client client =
        new Client(
            Config.builder()
                .baseUrl("http://127.0.0.1:8080")
                .projectRef("sdk-fixture")
                .transport(transport)
                .patcher(patcher)
                .build());
    UpdateCheckRequest req = new UpdateCheckRequest();
    req.currentVersion = "1.0.0";
    req.os = "windows";
    req.arch = "x86_64";
    client.check(req);
    JsonNode body = Json.MAPPER.readTree(new String(transport.last().body(), StandardCharsets.UTF_8));
    List<String> caps = Json.MAPPER.convertValue(body.get("capabilities"), new TypeReference<List<String>>() {});
    assertTrue(caps.contains("binary_delta"));
    List<String> algos = Json.MAPPER.convertValue(body.get("accepted_delta_algos"), new TypeReference<List<String>>() {});
    assertTrue(algos.contains("bsdiff"));
    assertTrue(algos.contains("xdelta3"));
    assertTrue(algos.contains("hdiffpatch"));
  }

  @Test
  void withoutUnpackerOrFileStoreCapabilitiesShrink() throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Client client =
        new Client(
            Config.builder()
                .baseUrl("http://127.0.0.1:8080")
                .projectRef("sdk-fixture")
                .transport(transport)
                .archiveUnpacker(null)
                .fileStore(null)
                .build());
    UpdateCheckRequest req = new UpdateCheckRequest();
    req.currentVersion = "1.0.0";
    req.os = "windows";
    req.arch = "x86_64";
    client.check(req);
    JsonNode body = Json.MAPPER.readTree(new String(transport.last().body(), StandardCharsets.UTF_8));
    List<String> caps = Json.MAPPER.convertValue(body.get("capabilities"), new TypeReference<List<String>>() {});
    assertTrue(caps.contains("full_package"));
    assertFalse(caps.contains("patch_package"));
    assertFalse(caps.contains("file_list"));
    assertFalse(caps.contains("binary_delta"));
  }
}
