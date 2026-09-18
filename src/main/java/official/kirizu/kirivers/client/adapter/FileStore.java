package official.kirizu.kirivers.client.adapter;

import java.io.IOException;
import java.nio.file.Path;
import java.util.List;

/**
 * Local tree used to stage downloads and compare integrity. Paths passed to
 * {@link #normalize(String)} must use {@code /} + NFC.
 */
public interface FileStore {

  /** When true, check may declare {@code file_list}. */
  boolean canWriteIndividualFiles();

  String normalize(String relativePath);

  boolean exists(Path path);

  byte[] read(Path path) throws IOException;

  void write(Path path, byte[] data) throws IOException;

  List<Path> listFiles(Path directory) throws IOException;

  String sha256(Path path) throws IOException;

  String md5(Path path) throws IOException;
}
