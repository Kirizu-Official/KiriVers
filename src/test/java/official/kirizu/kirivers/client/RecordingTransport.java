package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.HttpRequest;
import official.kirizu.kirivers.client.adapter.HttpResponse;
import official.kirizu.kirivers.client.adapter.Transport;

import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.function.Function;

/** In-memory Transport for contract tests. Does not open sockets or Docker. */
public final class RecordingTransport implements Transport {

  public final List<HttpRequest> requests = new ArrayList<>();
  private Function<HttpRequest, HttpResponse> handler = this::defaultHandler;

  public void handler(Function<HttpRequest, HttpResponse> handler) {
    this.handler = handler;
  }

  @Override
  public HttpResponse execute(HttpRequest request) {
    requests.add(request);
    return handler.apply(request);
  }

  public HttpRequest last() {
    return requests.get(requests.size() - 1);
  }

  public List<HttpRequest> ofMethodPath(String method, String pathContains) {
    List<HttpRequest> out = new ArrayList<>();
    for (HttpRequest r : requests) {
      if (r.method().equalsIgnoreCase(method) && r.url().contains(pathContains)) {
        out.add(r);
      }
    }
    return out;
  }

  public static HttpResponse json(int status, String body) {
    Map<String, List<String>> headers = new LinkedHashMap<>();
    headers.put("Content-Type", List.of("application/json"));
    return new HttpResponse(status, headers, body.getBytes(StandardCharsets.UTF_8));
  }

  public static HttpResponse bytes(int status, byte[] body) {
    Map<String, List<String>> headers = new LinkedHashMap<>();
    headers.put("Content-Type", List.of("application/octet-stream"));
    return new HttpResponse(status, headers, body);
  }

  private HttpResponse defaultHandler(HttpRequest request) {
    String url = request.url();
    String method = request.method().toUpperCase();
    if (url.endsWith("/api/v1/health") && "GET".equals(method)) {
      return json(200, "{\"status\":\"ok\",\"ready\":true}");
    }
    if (url.contains("/update/check") && "POST".equals(method)) {
      return json(
          200,
          "{\"has_update\":true,\"is_mandatory\":false,\"is_downgrade\":false,\"reason\":\"normal\","
              + "\"compare_engine\":\"semver\",\"version_integer\":null,\"version_semver\":\"1.1.0\","
              + "\"target_channel\":\"stable\",\"target_hw_rev\":null,\"package_type\":\"single_file\","
              + "\"root_hash\":\"\",\"package_url\":\"/api/v1/projects/sdk-fixture/packages/abc\","
              + "\"file_name\":\"app.bin\",\"size\":3,\"sha256\":\"aa\",\"delta_available\":false}");
    }
    if (url.contains("/update/pack") && "POST".equals(method)) {
      return json(200, "{\"status\":\"ready\",\"package_url\":\"/pkg\",\"sha256\":\"aa\",\"diff_mode\":\"patch_package\"}");
    }
    if (url.contains("/update/diff") && "POST".equals(method)) {
      return json(
          200,
          "{\"diff_mode\":\"full_package\",\"root_hash\":\"\",\"version_integer\":null,"
              + "\"version_semver\":\"1.1.0\",\"channel\":\"stable\",\"compare_engine\":\"semver\"}");
    }
    if (url.contains("/clients/report") && "POST".equals(method)) {
      return json(200, "{\"ip\":\"127.0.0.1\",\"country_code\":\"\",\"region_code\":\"\",\"geo_i18n\":{}}");
    }
    if (url.contains("/telemetry/report") && "POST".equals(method)) {
      return json(202, "{\"status\":\"accepted\"}");
    }
    if (url.contains("/integrity") && "GET".equals(method)) {
      return json(
          200,
          "{\"version_integer\":null,\"version_semver\":\"1.1.0\",\"channel\":\"stable\","
              + "\"package_type\":\"single_file\",\"root_hash\":\"\",\"full_package_url\":\"/pkg\","
              + "\"file_name\":\"app.bin\",\"size\":3,\"sha256\":\"aa\",\"files\":[]}");
    }
    if (url.contains("/changelog/") && "GET".equals(method)) {
      return json(200, "{\"changelog\":\"hi\",\"changelog_versions\":[]}");
    }
    if (url.contains("/announcements") && "GET".equals(method)) {
      return json(200, "{\"announcements\":[]}");
    }
    if (url.contains("/channels") && "GET".equals(method)) {
      return json(200, "{\"channels\":[]}");
    }
    if (url.contains("/matrix") && "GET".equals(method)) {
      return json(200, "{\"matrix\":[]}");
    }
    if (url.contains("/languages") && "GET".equals(method)) {
      return json(200, "{\"languages\":[]}");
    }
    if ((url.contains("/packages/") || url.contains("/media/")) && ("GET".equals(method) || "HEAD".equals(method))) {
      return bytes(200, new byte[] {1, 2, 3});
    }
    if (url.matches(".*/projects/[^/]+$") && "GET".equals(method)) {
      return json(200, "{\"slug\":\"sdk-fixture\",\"uuid\":\"00000000-0000-0000-0000-000000000000\"}");
    }
    return json(404, "{\"error\":{\"code\":\"NOT_FOUND\",\"message\":\"no mock\",\"details\":null}}");
  }
}
