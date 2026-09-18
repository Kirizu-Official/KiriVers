package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class ChangelogResponse {
  public String changelog;
  public List<ChangelogVersion> changelogVersions;
}
