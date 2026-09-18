package official.kirizu.kirivers.client.adapter;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardCopyOption;

/**
 * Optional desktop {@link Replacer} using {@link Files#move}. Not installed as
 * the SDK default — Android must supply its own implementation.
 */
public final class FilesMoveReplacer implements Replacer {

  @Override
  public void replace(Path staged, Path target) throws IOException {
    Path parent = target.getParent();
    if (parent != null) {
      Files.createDirectories(parent);
    }
    try {
      Files.move(staged, target, StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING);
    } catch (IOException atomicUnsupported) {
      Files.move(staged, target, StandardCopyOption.REPLACE_EXISTING);
    }
  }
}
