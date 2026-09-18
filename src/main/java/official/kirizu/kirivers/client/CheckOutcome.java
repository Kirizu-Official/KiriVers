package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.HttpResponse;
import official.kirizu.kirivers.client.model.UpdateCheck200;

/** POST /update/check: 200 update, 204 no update, 304 ETag hit. 204 is not an error. */
public final class CheckOutcome {

  public enum Status {
    UPDATE,
    NO_UPDATE,
    NOT_MODIFIED
  }

  private final Status status;
  private final UpdateCheck200 body;
  private final String etag;
  private final HttpResponse raw;

  public CheckOutcome(Status status, UpdateCheck200 body, String etag, HttpResponse raw) {
    this.status = status;
    this.body = body;
    this.etag = etag;
    this.raw = raw;
  }

  public Status status() {
    return status;
  }

  public boolean hasUpdate() {
    return status == Status.UPDATE && body != null && Boolean.TRUE.equals(body.hasUpdate);
  }

  public UpdateCheck200 body() {
    return body;
  }

  public String etag() {
    return etag;
  }

  public HttpResponse raw() {
    return raw;
  }
}
