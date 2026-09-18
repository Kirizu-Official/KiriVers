package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class ProjectPublic {
  public String uuid;
  public String slug;
  public String compareEngine;
  public String createdAt;
  public String updatedAt;
  public String defaultLocale;
  public String deviceIdPolicy;
  public Boolean forceHttps;
  public String minimumSupportedVersion;
  public Boolean requireClientToken;
  public String storageVisibility;
}
