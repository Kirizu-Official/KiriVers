package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonInclude;

@JsonIgnoreProperties(ignoreUnknown = true)
@JsonInclude(JsonInclude.Include.NON_EMPTY)
public final class TelemetryRequest {
  public String os;
  public String arch;
  public String channel;
  public String fromVersion;
  public String toVersion;
  public String status;
  public String deviceId;
  public String diffMode;
  public String errorCode;
  public String errorMessage;
}
