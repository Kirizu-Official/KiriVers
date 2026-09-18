package official.kirizu.kirivers.client.adapter;

import java.io.IOException;
import java.nio.file.Path;

/**
 * Install-time replace. Desktop callers may use {@link FilesMoveReplacer}.
 * Android / HarmonyOS APK install is the app's {@code PackageInstaller} (or
 * equivalent) — this SDK does not bundle JNI or a default installer.
 */
public interface Replacer {

  void replace(Path staged, Path target) throws IOException;
}
