package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class UpdateCheck200 {
  public Boolean hasUpdate;
  public Boolean isMandatory;
  public Boolean isDowngrade;
  public String reason;
  public String compareEngine;
  public Long versionInteger;
  public String versionSemver;
  public String targetChannel;
  public String targetHwRev;
  public String packageType;
  public String publishTime;
  public String platformNotes;
  public String rootHash;
  public String packageUrl;
  public String fileName;
  public Long size;
  public String sha256;
  public Boolean deltaAvailable;
  public String deltaAlgo;
  public String signature;
  public String artifactSignature;
}
