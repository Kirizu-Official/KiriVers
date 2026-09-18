package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class FileEntry {
  public String path;
  public Long size;
  public String sha256;
  public String md5;
  public String url;
  public String installPolicy;
  public Boolean integrityCheck;
}
