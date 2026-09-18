package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.HttpResponse;

/** GET/HEAD bytes (packages or media). Status may be 200 or 206. */
public final class BinaryOutcome {

  private final int httpStatus;
  private final byte[] body;
  private final HttpResponse raw;

  public BinaryOutcome(int httpStatus, byte[] body, HttpResponse raw) {
    this.httpStatus = httpStatus;
    this.body = body == null ? new byte[0] : body;
    this.raw = raw;
  }

  public int httpStatus() {
    return httpStatus;
  }

  public byte[] body() {
    return body;
  }

  public HttpResponse raw() {
    return raw;
  }
}
