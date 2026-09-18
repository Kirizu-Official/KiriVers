package official.kirizu.kirivers.client.adapter;

import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Path;

/** SHA-256 (and MD5 when integrity asks). */
public interface Hasher {

  String sha256(byte[] data);

  String sha256(InputStream in) throws IOException;

  String sha256(Path file) throws IOException;

  String md5(byte[] data);

  String md5(InputStream in) throws IOException;

  String md5(Path file) throws IOException;
}
