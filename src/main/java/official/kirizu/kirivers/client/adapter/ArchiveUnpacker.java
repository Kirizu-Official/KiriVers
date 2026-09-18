package official.kirizu.kirivers.client.adapter;

import java.io.IOException;
import java.nio.file.Path;
import java.util.Map;

/**
 * Unpack a native zip whose members are named by content hash. Values in
 * {@code hashToRelativePath} are install-relative paths ({@code /} + NFC).
 */
public interface ArchiveUnpacker {

  void unpack(byte[] archive, Map<String, String> hashToRelativePath, Path destDir) throws IOException;
}
