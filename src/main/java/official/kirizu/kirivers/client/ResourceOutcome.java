package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.HttpResponse;

/** JSON GET that may return 304. */
public final class ResourceOutcome<T> {

  private final int httpStatus;
  private final T body;
  private final String etag;
  private final HttpResponse raw;

  public ResourceOutcome(int httpStatus, T body, String etag, HttpResponse raw) {
    this.httpStatus = httpStatus;
    this.body = body;
    this.etag = etag;
    this.raw = raw;
  }

  public int httpStatus() {
    return httpStatus;
  }

  public boolean notModified() {
    return httpStatus == 304;
  }

  public T body() {
    return body;
  }

  public String etag() {
    return etag;
  }

  public HttpResponse raw() {
    return raw;
  }
}
