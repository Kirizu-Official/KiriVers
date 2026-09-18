package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.model.Pack200;

public final class PackOutcome {

  private final int httpStatus;
  private final Pack200 body;

  public PackOutcome(int httpStatus, Pack200 body) {
    this.httpStatus = httpStatus;
    this.body = body;
  }

  public int httpStatus() {
    return httpStatus;
  }

  public Pack200 body() {
    return body;
  }

  public boolean pending() {
    return httpStatus == 202 || (body != null && "pending".equalsIgnoreCase(body.status));
  }

  public boolean ready() {
    return body != null && "ready".equalsIgnoreCase(body.status);
  }

  public boolean fullPackage() {
    return body != null && "full_package".equalsIgnoreCase(body.status);
  }
}
