package official.kirizu.kirivers.client;

import com.fasterxml.jackson.databind.JsonNode;
import official.kirizu.kirivers.client.adapter.HttpRequest;
import official.kirizu.kirivers.client.model.AnnouncementsQuery;
import official.kirizu.kirivers.client.model.ChangelogQuery;
import official.kirizu.kirivers.client.model.ClientLoginInput;
import official.kirizu.kirivers.client.model.DiffRequest;
import official.kirizu.kirivers.client.model.IntegrityQuery;
import official.kirizu.kirivers.client.model.PackRequest;
import official.kirizu.kirivers.client.model.TelemetryRequest;
import official.kirizu.kirivers.client.model.UpdateCheckRequest;
import org.junit.jupiter.api.Test;

import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Iterator;
import java.util.LinkedHashSet;
import java.util.Map;
import java.util.Set;
import java.util.TreeSet;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ContractTest {

  private static final String PROJECT = "sdk-fixture";
  private static final String MEDIA = "11111111-1111-1111-1111-111111111111";

  @Test
  void handwrittenClientCoversNativeOpenApiOperations() throws Exception {
    JsonNode spec = Json.MAPPER.readTree(openapiBytes());
    Set<String> fromSpec = nativeOps(spec);

    RecordingTransport transport = new RecordingTransport();
    Client client = new Client(Config.builder().baseUrl("http://127.0.0.1:8080").projectRef(PROJECT).transport(transport).build());

    client.health();
    client.project();
    ClientLoginInput login = new ClientLoginInput();
    login.deviceId = "dev-1";
    login.os = "windows";
    login.arch = "x86_64";
    client.deviceReport(login);

    UpdateCheckRequest check = new UpdateCheckRequest();
    check.currentVersion = "1.0.0";
    check.os = "windows";
    check.arch = "x86_64";
    check.channel = "stable";
    client.check(check, "\"etag\"");

    ChangelogQuery cq = new ChangelogQuery();
    cq.fromVersion = "1.0.0";
    client.changelog("stable", "windows", "x86_64", cq);

    IntegrityQuery iq = new IntegrityQuery();
    iq.os = "windows";
    iq.arch = "x86_64";
    iq.hashAlgo = "sha256";
    client.integrity("1.1.0", iq);

    DiffRequest diff = new DiffRequest();
    diff.sourceVersion = "1.0.0";
    diff.targetVersion = "1.1.0";
    diff.os = "windows";
    diff.arch = "x86_64";
    diff.localSha256 = "c89e";
    client.diff(diff);

    PackRequest pack = new PackRequest();
    pack.sourceVersion = "1.0.0";
    pack.targetVersion = "1.1.0";
    pack.os = "windows";
    pack.arch = "x86_64";
    pack.neededPaths = java.util.List.of("bin/app");
    client.pack(pack);

    client.downloadPackage("abc", "1", "sig", "bytes=0-1");
    client.headPackage("abc", "1", "sig");
    client.channels();
    client.matrix();
    client.languages();
    client.announcements(new AnnouncementsQuery());
    TelemetryRequest tel = new TelemetryRequest();
    tel.os = "windows";
    tel.arch = "x86_64";
    tel.channel = "stable";
    tel.fromVersion = "1.0.0";
    tel.toVersion = "1.1.0";
    tel.status = "installed";
    client.reportTelemetry(tel);
    client.downloadMedia(MEDIA, null);
    client.headMedia(MEDIA);

    Set<String> seen = new TreeSet<>();
    for (HttpRequest r : transport.requests) {
      seen.add(r.method().toUpperCase() + " " + toTemplate(r.url()));
      String url = r.url();
      assertFalse(url.contains("/store/"), url);
      assertFalse(url.contains("/clients/login"), url);
      assertFalse(url.contains("/pack/status"), url);
      assertFalse(url.contains("/manifest"), url);
      assertFalse(url.contains("/artifacts/"), url);
      assertFalse(url.matches(".*GET$"), url);
    }
    assertFalse(seen.contains("GET /api/v1/projects/{project_ref}/update/check"));
    assertEquals(fromSpec, seen, "handwritten client drifted from openapi.client.json");

    HttpRequest checkReq = transport.ofMethodPath("POST", "/update/check").get(0);
    JsonNode checkBody = Json.MAPPER.readTree(checkReq.body());
    assertTrue(checkBody.has("current_version"));
    assertTrue(checkBody.has("os"));
    assertTrue(checkBody.has("arch"));
    assertFalse(checkBody.has("local_sha256"));
    assertFalse(checkBody.has("dirty_paths"));
    assertTrue(checkReq.headers().containsKey("If-None-Match"));

    HttpRequest integ = transport.ofMethodPath("GET", "/integrity").get(0);
    String q = URI.create(integ.url()).getQuery();
    assertTrue(q.contains("os="));
    assertTrue(q.contains("arch="));

    HttpRequest pkg = transport.ofMethodPath("GET", "/packages/").get(0);
    assertTrue(pkg.url().contains("exp="));
    assertTrue(pkg.url().contains("sig="));
    assertEquals("bytes=0-1", pkg.headers().get("Range"));
  }

  @Test
  void errorEnvelopeSurfacesStableCode() {
    RecordingTransport transport = new RecordingTransport();
    transport.handler(
        r ->
            RecordingTransport.json(
                404, "{\"error\":{\"code\":\"PROJECT_NOT_FOUND\",\"message\":\"gone\",\"details\":{\"slug\":\"x\"}}}"));
    Client client = new Client(Config.builder().baseUrl("http://127.0.0.1:8080").projectRef(PROJECT).transport(transport).build());
    ApiException ex = assertThrows(ApiException.class, client::project);
    assertEquals(404, ex.httpStatus());
    assertEquals("PROJECT_NOT_FOUND", ex.code());
    assertEquals("gone", ex.getMessage());
    assertNotNull(ex.details());
  }

  @Test
  void check204IsNotAnError() {
    RecordingTransport transport = new RecordingTransport();
    transport.handler(r -> RecordingTransport.json(204, ""));
    Client client = new Client(Config.builder().baseUrl("http://127.0.0.1:8080").projectRef(PROJECT).transport(transport).build());
    UpdateCheckRequest req = new UpdateCheckRequest();
    req.currentVersion = "1.1.0";
    req.os = "windows";
    req.arch = "x86_64";
    CheckOutcome out = client.check(req);
    assertEquals(CheckOutcome.Status.NO_UPDATE, out.status());
  }

  @Test
  void check304IsEtagHitNotAnError() {
    RecordingTransport transport = new RecordingTransport();
    transport.handler(
        r -> {
          java.util.Map<String, java.util.List<String>> headers = new java.util.LinkedHashMap<>();
          headers.put("ETag", java.util.List.of("\"abc\""));
          return new official.kirizu.kirivers.client.adapter.HttpResponse(304, headers, new byte[0]);
        });
    Client client = new Client(Config.builder().baseUrl("http://127.0.0.1:8080").projectRef(PROJECT).transport(transport).build());
    UpdateCheckRequest req = new UpdateCheckRequest();
    req.currentVersion = "1.1.0";
    req.os = "windows";
    req.arch = "x86_64";
    CheckOutcome out = client.check(req, "\"abc\"");
    assertEquals(CheckOutcome.Status.NOT_MODIFIED, out.status());
    assertEquals("\"abc\"", out.etag());
  }

  @Test
  void requiredFieldsOnNativeBodies() throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Client client = new Client(Config.builder().baseUrl("http://example").projectRef(PROJECT).transport(transport).build());

    ClientLoginInput login = new ClientLoginInput();
    login.deviceId = "dev-1";
    client.deviceReport(login);
    JsonNode report = Json.MAPPER.readTree(transport.last().body());
    assertTrue(report.has("device_id"));
    assertEquals("dev-1", report.get("device_id").asText());

    DiffRequest diff = new DiffRequest();
    diff.sourceVersion = "1.0.0";
    diff.targetVersion = "1.1.0";
    diff.os = "windows";
    diff.arch = "x86_64";
    diff.localSha256 = "aa";
    client.diff(diff);
    JsonNode diffBody = Json.MAPPER.readTree(transport.last().body());
    assertTrue(diffBody.has("source_version"));
    assertTrue(diffBody.has("target_version"));
    assertTrue(diffBody.has("os"));
    assertTrue(diffBody.has("arch"));
    assertTrue(diffBody.has("local_sha256"));

    PackRequest pack = new PackRequest();
    pack.sourceVersion = "1.0.0";
    pack.targetVersion = "1.1.0";
    pack.os = "windows";
    pack.arch = "x86_64";
    client.pack(pack);
    JsonNode packBody = Json.MAPPER.readTree(transport.last().body());
    assertTrue(packBody.has("source_version"));
    assertTrue(packBody.has("target_version"));
    assertTrue(packBody.has("os"));
    assertTrue(packBody.has("arch"));
    assertTrue(packBody.has("needed_paths"));
    assertTrue(packBody.get("needed_paths").isArray());

    TelemetryRequest tel = new TelemetryRequest();
    tel.os = "windows";
    tel.arch = "x86_64";
    tel.channel = "stable";
    tel.fromVersion = "1.0.0";
    tel.toVersion = "1.1.0";
    tel.status = "installed";
    client.reportTelemetry(tel);
    JsonNode telBody = Json.MAPPER.readTree(transport.last().body());
    assertTrue(telBody.has("os"));
    assertTrue(telBody.has("arch"));
    assertTrue(telBody.has("channel"));
    assertTrue(telBody.has("from_version"));
    assertTrue(telBody.has("to_version"));
    assertTrue(telBody.has("status"));
  }

  private static Set<String> nativeOps(JsonNode spec) {
    Set<String> out = new TreeSet<>();
    Iterator<Map.Entry<String, JsonNode>> paths = spec.get("paths").fields();
    while (paths.hasNext()) {
      Map.Entry<String, JsonNode> e = paths.next();
      String path = e.getKey();
      if (path.contains("/store/") || path.endsWith("/openapi.json")) {
        continue;
      }
      Iterator<String> methods = e.getValue().fieldNames();
      while (methods.hasNext()) {
        String m = methods.next();
        if ("parameters".equals(m) || "options".equalsIgnoreCase(m) || "description".equals(m) || "summary".equals(m)) {
          continue;
        }
        String method = m.toUpperCase();
        if ("GET".equals(method) || "POST".equals(method) || "HEAD".equals(method)) {
          out.add(method + " " + path);
        }
      }
    }
    return out;
  }

  private static String toTemplate(String url) {
    URI u = URI.create(url);
    String path = u.getPath();
    path = path.replace("/projects/" + PROJECT, "/projects/{project_ref}");
    path = path.replace("/changelog/stable/windows/x86_64", "/changelog/{channel}/{os}/{arch}");
    path = path.replace("/versions/1.1.0/integrity", "/versions/{version}/integrity");
    path = path.replace("/packages/abc", "/packages/{ref}");
    path = path.replace("/media/" + MEDIA, "/media/{id}");
    return path;
  }

  private static byte[] openapiBytes() throws Exception {
    Path p = Path.of("openapi.client.json");
    assertTrue(Files.isRegularFile(p), "openapi.client.json must live at the package root");
    return Files.readAllBytes(p);
  }

  @Test
  void checkBodyUsesSpecPropertyNames() throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Client client = new Client(Config.builder().baseUrl("http://example").projectRef(PROJECT).transport(transport).build());
    UpdateCheckRequest req = new UpdateCheckRequest();
    req.currentVersion = "1.0.0";
    req.os = "windows";
    req.arch = "x86_64";
    req.channel = "stable";
    req.deviceId = "dev-1";
    client.check(req);
    JsonNode body = Json.MAPPER.readTree(new String(transport.last().body(), StandardCharsets.UTF_8));
    assertEquals("1.0.0", body.get("current_version").asText());
    assertEquals("windows", body.get("os").asText());
    assertEquals("x86_64", body.get("arch").asText());
    Set<String> names = new LinkedHashSet<>();
    body.fieldNames().forEachRemaining(names::add);
    assertFalse(names.contains("currentVersion"));
  }
}
