package official.kirizu.kirivers.client.adapter;

import official.kirizu.kirivers.client.PathUtil;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Locale;
import java.util.Map;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;

/** Default {@link ArchiveUnpacker} using {@code java.util.zip}. */
public final class ZipArchiveUnpacker implements ArchiveUnpacker {

  @Override
  public void unpack(byte[] archive, Map<String, String> hashToRelativePath, Path destDir) throws IOException {
    if (archive == null) {
      throw new IOException("archive is required");
    }
    Path root = destDir.toAbsolutePath().normalize();
    Files.createDirectories(root);
    try (ZipInputStream zin = new ZipInputStream(new ByteArrayInputStream(archive))) {
      ZipEntry entry;
      while ((entry = zin.getNextEntry()) != null) {
        if (entry.isDirectory()) {
          continue;
        }
        String name = entry.getName();
        int slash = name.lastIndexOf('/');
        String member = slash >= 0 ? name.substring(slash + 1) : name;
        String key = stripExt(member).toLowerCase(Locale.ROOT);
        String rel = lookup(hashToRelativePath, key);
        if (rel == null) {
          rel = PathUtil.normalize(member);
        } else {
          rel = PathUtil.normalize(rel);
        }
        Path dest = root;
        for (String part : rel.split("/")) {
          if (part.isEmpty() || ".".equals(part)) {
            continue;
          }
          Path segment = Path.of(part);
          if (segment.isAbsolute() || "..".equals(part)) {
            throw new IOException("zip slip: " + entry.getName());
          }
          dest = dest.resolve(part);
        }
        dest = dest.normalize();
        if (!dest.startsWith(root) || dest.equals(root)) {
          throw new IOException("zip slip: " + entry.getName());
        }
        Path parent = dest.getParent();
        if (parent != null) {
          Files.createDirectories(parent);
        }
        Files.copy(zin, dest, java.nio.file.StandardCopyOption.REPLACE_EXISTING);
        zin.closeEntry();
      }
    }
  }

  private static String stripExt(String name) {
    int dot = name.lastIndexOf('.');
    return dot > 0 ? name.substring(0, dot) : name;
  }

  private static String lookup(Map<String, String> map, String sha) {
    if (map == null) {
      return null;
    }
    for (Map.Entry<String, String> e : map.entrySet()) {
      if (e.getKey() != null && e.getKey().toLowerCase(Locale.ROOT).equals(sha)) {
        return e.getValue();
      }
    }
    return null;
  }
}
