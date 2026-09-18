package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import java.util.Map;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class ClientReportOutput {
  public String ip;
  public String countryCode;
  public String regionCode;
  public Map<String, Object> geoI18n;
}
