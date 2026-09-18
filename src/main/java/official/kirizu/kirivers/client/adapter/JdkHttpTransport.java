package official.kirizu.kirivers.client.adapter;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest.BodyPublishers;
import java.net.http.HttpResponse.BodyHandlers;
import java.time.Duration;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;

/** Default {@link Transport} using JDK 11+ {@link HttpClient}. Not OkHttp. */
public final class JdkHttpTransport implements Transport {

  private final HttpClient http;
  private final Duration requestTimeout;

  public JdkHttpTransport() {
    this(HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(10)).build(), Duration.ofSeconds(60));
  }

  public JdkHttpTransport(HttpClient http, Duration requestTimeout) {
    this.http = Objects.requireNonNull(http, "http");
    this.requestTimeout = requestTimeout == null ? Duration.ofSeconds(60) : requestTimeout;
  }

  @Override
  public HttpResponse execute(HttpRequest request) throws IOException {
    java.net.http.HttpRequest.Builder b =
        java.net.http.HttpRequest.newBuilder(URI.create(request.url())).timeout(requestTimeout);
    for (Map.Entry<String, String> h : request.headers().entrySet()) {
      if (h.getValue() != null) {
        b.header(h.getKey(), h.getValue());
      }
    }
    String method = request.method().toUpperCase(Locale.ROOT);
    byte[] body = request.body();
    if ("GET".equals(method) || "HEAD".equals(method)) {
      b.method(method, BodyPublishers.noBody());
    } else if (body == null || body.length == 0) {
      b.method(method, BodyPublishers.noBody());
    } else {
      b.method(method, BodyPublishers.ofByteArray(body));
    }
    try {
      java.net.http.HttpResponse<byte[]> resp = http.send(b.build(), BodyHandlers.ofByteArray());
      Map<String, List<String>> headers = new LinkedHashMap<>();
      resp.headers().map().forEach(headers::put);
      return new HttpResponse(resp.statusCode(), headers, resp.body());
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
      throw new IOException("HTTP request interrupted", e);
    }
  }
}
