package official.kirizu.kirivers.client.adapter;

import official.kirizu.kirivers.client.PathUtil;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.stream.Stream;

/** Default {@link FileStore} on {@code java.nio.file}. */
public final class NioFileStore implements FileStore {

  private final Hasher hasher;

  public NioFileStore() {
    this(new MessageDigestHasher());
  }

  public NioFileStore(Hasher hasher) {
    this.hasher = hasher == null ? new MessageDigestHasher() : hasher;
  }

  @Override
  public boolean canWriteIndividualFiles() {
    return true;
  }

  @Override
  public String normalize(String relativePath) {
    return PathUtil.normalize(relativePath);
  }

  @Override
  public boolean exists(Path path) {
    return path != null && Files.isRegularFile(path);
  }

  @Override
  public byte[] read(Path path) throws IOException {
    return Files.readAllBytes(path);
  }

  @Override
  public void write(Path path, byte[] data) throws IOException {
    Path parent = path.getParent();
    if (parent != null) {
      Files.createDirectories(parent);
    }
    Files.write(path, data == null ? new byte[0] : data);
  }

  @Override
  public List<Path> listFiles(Path directory) throws IOException {
    if (directory == null || !Files.isDirectory(directory)) {
      return List.of();
    }
    List<Path> out = new ArrayList<>();
    try (Stream<Path> walk = Files.walk(directory)) {
      walk.filter(Files::isRegularFile).forEach(out::add);
    }
    return out;
  }

  @Override
  public String sha256(Path path) throws IOException {
    return hasher.sha256(path);
  }

  @Override
  public String md5(Path path) throws IOException {
    return hasher.md5(path);
  }
}
