package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonInclude;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
@JsonInclude(JsonInclude.Include.NON_EMPTY)
public final class PackRequest {
  public String sourceVersion;
  public String targetVersion;
  public String os;
  public String arch;
  public String channel;
  public String hwRev;
  public String deviceId;

  /** Always emitted (including {@code []}) so pack poll identity stays stable. */
  @JsonInclude(JsonInclude.Include.ALWAYS)
  public List<String> neededPaths;
}
