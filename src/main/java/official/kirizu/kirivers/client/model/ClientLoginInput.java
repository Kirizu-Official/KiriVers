package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonInclude;
import java.util.Map;

@JsonIgnoreProperties(ignoreUnknown = true)
@JsonInclude(JsonInclude.Include.NON_EMPTY)
public final class ClientLoginInput {
  public String deviceId;
  public String version;
  public String os;
  public String arch;
  public String channel;
  public Map<String, Object> custom;
}
