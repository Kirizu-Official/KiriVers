package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class Diff200 {
  public String diffMode;
  public String rootHash;
  public Long versionInteger;
  public String versionSemver;
  public String channel;
  public String compareEngine;
  public String packageUrl;
  public String fileName;
  public Long size;
  public String sha256;
  public String signature;
  public String deltaAlgo;
  public List<String> deletedPaths;
  public List<String> invalidPaths;
  public List<FileEntry> files;
}
