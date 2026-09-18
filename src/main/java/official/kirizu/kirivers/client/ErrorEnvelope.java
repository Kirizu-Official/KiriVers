package official.kirizu.kirivers.client;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

/** Unified error envelope {@code { "error": { "code", "message", "details" } }}. */
@JsonIgnoreProperties(ignoreUnknown = true)
public final class ErrorEnvelope {

  public ErrorBody error;

  @JsonIgnoreProperties(ignoreUnknown = true)
  public static final class ErrorBody {
    public String code;
    public String message;
    public Object details;
  }

  public String code() {
    return error == null ? null : error.code;
  }

  public String message() {
    return error == null ? null : error.message;
  }

  public Object details() {
    return error == null ? null : error.details;
  }

  public static ErrorEnvelope of(String code, String message, Object details) {
    ErrorEnvelope env = new ErrorEnvelope();
    env.error = new ErrorBody();
    env.error.code = code;
    env.error.message = message;
    env.error.details = details;
    return env;
  }

  public static boolean looksLike(ErrorEnvelope env) {
    return env != null && env.error != null && env.error.code != null && !env.error.code.isBlank();
  }
}
