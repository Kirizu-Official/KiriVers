package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class ClientChannel {
  public String name;
  public String slug;
  public Integer stabilityRank;
}
