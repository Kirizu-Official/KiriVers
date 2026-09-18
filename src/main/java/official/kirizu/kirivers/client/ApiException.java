package official.kirizu.kirivers.client;

import java.util.Optional;

/**
 * HTTP error from the client plane. {@code code} is the stable envelope token
 * (or the raw string if the server sent an unknown code).
 */
public final class ApiException extends RuntimeException {

  private final int httpStatus;
  private final String code;
  private final Object details;
  private final Integer retryAfterSeconds;

  public ApiException(int httpStatus, String code, String message, Object details, Integer retryAfterSeconds) {
    this(httpStatus, code, message, details, retryAfterSeconds, null);
  }

  public ApiException(
      int httpStatus, String code, String message, Object details, Integer retryAfterSeconds, Throwable cause) {
    super(message == null || message.isBlank() ? (code == null ? "HTTP " + httpStatus : code) : message, cause);
    this.httpStatus = httpStatus;
    this.code = code == null ? "HTTP_" + httpStatus : code;
    this.details = details;
    this.retryAfterSeconds = retryAfterSeconds;
  }

  public int httpStatus() {
    return httpStatus;
  }

  public String code() {
    return code;
  }

  public Object details() {
    return details;
  }

  public Optional<Integer> retryAfterSeconds() {
    return Optional.ofNullable(retryAfterSeconds);
  }
}
