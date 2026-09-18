package official.kirizu.kirivers.client.adapter;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;

/** One HTTP response. Header lookup is case-insensitive. */
public final class HttpResponse {

  private final int status;
  private final Map<String, List<String>> headers;
  private final byte[] body;

  public HttpResponse(int status, Map<String, List<String>> headers, byte[] body) {
    this.status = status;
    Map<String, List<String>> copy = new LinkedHashMap<>();
    if (headers != null) {
      for (Map.Entry<String, List<String>> e : headers.entrySet()) {
        if (e.getKey() == null) {
          continue;
        }
        copy.put(e.getKey(), List.copyOf(e.getValue() == null ? List.of() : e.getValue()));
      }
    }
    this.headers = Map.copyOf(copy);
    this.body = body == null ? new byte[0] : body;
  }

  public int status() {
    return status;
  }

  public Map<String, List<String>> headers() {
    return headers;
  }

  public byte[] body() {
    return body;
  }

  public String header(String name) {
    Objects.requireNonNull(name, "name");
    List<String> values = headerValues(name);
    return values.isEmpty() ? null : values.get(0);
  }

  public List<String> headerValues(String name) {
    String want = name.toLowerCase(Locale.ROOT);
    for (Map.Entry<String, List<String>> e : headers.entrySet()) {
      if (e.getKey().toLowerCase(Locale.ROOT).equals(want)) {
        return e.getValue();
      }
    }
    return List.of();
  }

  public Map<String, List<String>> headersLowercased() {
    Map<String, List<String>> out = new LinkedHashMap<>();
    for (Map.Entry<String, List<String>> e : headers.entrySet()) {
      out.computeIfAbsent(e.getKey().toLowerCase(Locale.ROOT), k -> new ArrayList<>()).addAll(e.getValue());
    }
    return out;
  }
}
