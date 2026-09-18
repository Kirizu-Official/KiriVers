package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class ClientAnnouncement {
  public String id;
  public String locale;
  public String title;
  public String subtitle;
  public String markdown;
  public String startsAt;
  public String endsAt;
}
