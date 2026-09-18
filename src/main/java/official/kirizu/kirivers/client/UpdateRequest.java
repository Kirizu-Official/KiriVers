package official.kirizu.kirivers.client;

import java.nio.file.Path;
import java.util.Map;

/** Per-call inputs for {@link Updater}. Identity fields are caller-supplied (D14). */
public final class UpdateRequest {

  public String currentVersion;
  public String os;
  public String arch;
  public String channel;
  public String deviceId;
  public String osVersion;
  public String hwRev;
  public Map<String, Object> custom;
  public boolean reportDevice;
  public String ifNoneMatch;
  /** Local single-file bytes used for binary delta ({@code local_sha256} on diff only). */
  public byte[] localFile;
  public String localSha256;
  /** Directory of an existing multi-file install, used to compute {@code needed_paths}. */
  public Path installDir;
  /** Where verified bytes are written. Required for download. */
  public Path stageDir;
  /** Final install location for {@link official.kirizu.kirivers.client.adapter.Replacer}. */
  public Path targetPath;
}
