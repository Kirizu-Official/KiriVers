package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class Integrity200 {
  public Long versionInteger;
  public String versionSemver;
  public String channel;
  public String packageType;
  public String rootHash;
  public String fullPackageUrl;
  public String fileName;
  public Long size;
  public String sha256;
  public String signature;
  public List<FileEntry> files;
  public List<VolumeEntry> volumes;
}
