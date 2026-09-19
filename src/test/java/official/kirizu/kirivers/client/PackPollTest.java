package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.HttpRequest;
import official.kirizu.kirivers.client.model.PackRequest;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class PackPollTest {

  @Test
  void packUntilReadyRepeatsIdenticalJson() {
    RecordingTransport transport = new RecordingTransport();
    AtomicInteger n = new AtomicInteger();
    transport.handler(
        req -> {
          int i = n.getAndIncrement();
          if (i == 0) {
            return RecordingTransport.json(202, "{\"status\":\"pending\"}");
          }
          return RecordingTransport.json(
              200, "{\"status\":\"ready\",\"package_url\":\"/pkg\",\"sha256\":\"aa\",\"diff_mode\":\"patch_package\"}");
        });
    Client client =
        new Client(
            Config.builder()
                .baseUrl("http://127.0.0.1:8080")
                .projectRef("sdk-fixture")
                .transport(transport)
                .packPollInitial(java.time.Duration.ofMillis(5))
                .packPollMax(java.time.Duration.ofMillis(5))
                .build());
    PackRequest req = new PackRequest();
    req.sourceVersion = "1.0.0";
    req.targetVersion = "1.1.0";
    req.os = "windows";
    req.arch = "x86_64";
    req.neededPaths = List.of("b", "a");
    PackOutcome out = client.packUntilReady(req);
    assertTrue(out.ready());
    List<HttpRequest> packs = transport.ofMethodPath("POST", "/update/pack");
    assertEquals(2, packs.size());
    assertArrayEquals(packs.get(0).body(), packs.get(1).body());
    String json = new String(packs.get(0).body(), StandardCharsets.UTF_8);
    assertTrue(json.contains("needed_paths"));
    assertTrue(json.contains("source_version"));
    assertTrue(json.contains("target_version"));
  }

  @Test
  void emptyNeededPathsAreSerialized() {
    RecordingTransport transport = new RecordingTransport();
    Client client =
        new Client(Config.builder().baseUrl("http://127.0.0.1:8080").projectRef("sdk-fixture").transport(transport).build());
    PackRequest req = new PackRequest();
    req.sourceVersion = "1.0.0";
    req.targetVersion = "1.1.0";
    req.os = "windows";
    req.arch = "x86_64";
    client.pack(req);
    String json = new String(transport.last().body(), StandardCharsets.UTF_8);
    assertTrue(json.contains("\"needed_paths\":[]") || json.contains("\"needed_paths\": []"));
  }
}
