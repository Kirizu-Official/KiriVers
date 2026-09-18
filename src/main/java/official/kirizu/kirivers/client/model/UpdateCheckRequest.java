package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonInclude;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
@JsonInclude(JsonInclude.Include.NON_EMPTY)
public final class UpdateCheckRequest {
  public String currentVersion;
  public String os;
  public String arch;
  public String channel;
  public String hwRev;
  public String osVersion;
  public String deviceId;
  public List<String> capabilities;
  public List<String> acceptedDeltaAlgos;
}
