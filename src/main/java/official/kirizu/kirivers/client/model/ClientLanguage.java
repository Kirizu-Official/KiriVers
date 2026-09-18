package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class ClientLanguage {
  public String code;
  public String displayName;
  public Boolean isDefault;
  public Integer sortOrder;
}
