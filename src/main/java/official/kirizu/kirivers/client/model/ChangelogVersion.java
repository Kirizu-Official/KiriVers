package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class ChangelogVersion {
  public String changelog;
  public String channel;
  public Boolean hadArtifactForRequestPlatform;
  public String platformNotes;
  public String status;
  public String title;
  public Long versionInteger;
  public String versionSemver;
}
