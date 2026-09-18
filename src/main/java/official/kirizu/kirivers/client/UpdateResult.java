package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.model.UpdateCheck200;

import java.nio.file.Path;

/** Result of {@link Updater#update}. Missing Replacer stages files instead of failing. */
public final class UpdateResult {

  public enum Status {
    UP_TO_DATE,
    NOT_MODIFIED,
    STAGED,
    REPLACED
  }

  private final Status status;
  private final CheckOutcome check;
  private final Path stagedPath;
  private final String sha256;
  private final String diffMode;
  private final boolean replaced;

  public UpdateResult(
      Status status, CheckOutcome check, Path stagedPath, String sha256, String diffMode, boolean replaced) {
    this.status = status;
    this.check = check;
    this.stagedPath = stagedPath;
    this.sha256 = sha256;
    this.diffMode = diffMode;
    this.replaced = replaced;
  }

  public Status status() {
    return status;
  }

  public CheckOutcome check() {
    return check;
  }

  public UpdateCheck200 update() {
    return check == null ? null : check.body();
  }

  public Path stagedPath() {
    return stagedPath;
  }

  public String sha256() {
    return sha256;
  }

  public String diffMode() {
    return diffMode;
  }

  public boolean replaced() {
    return replaced;
  }
}
