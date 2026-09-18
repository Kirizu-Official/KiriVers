package official.kirizu.kirivers.client.adapter;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

/** One outbound HTTP call. Query strings must already be on {@link #url}. */
public final class HttpRequest {

  private final String method;
  private final String url;
  private final Map<String, String> headers;
  private final byte[] body;

  public HttpRequest(String method, String url, Map<String, String> headers, byte[] body) {
    this.method = Objects.requireNonNull(method, "method");
    this.url = Objects.requireNonNull(url, "url");
    this.headers = headers == null ? Map.of() : Map.copyOf(new LinkedHashMap<>(headers));
    this.body = body;
  }

  public String method() {
    return method;
  }

  public String url() {
    return url;
  }

  public Map<String, String> headers() {
    return headers;
  }

  public byte[] body() {
    return body;
  }
}
