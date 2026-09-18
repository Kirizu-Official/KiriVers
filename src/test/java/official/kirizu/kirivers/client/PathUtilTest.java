package official.kirizu.kirivers.client;

import org.junit.jupiter.api.Test;

import java.text.Normalizer;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class PathUtilTest {

  @Test
  void backslashToSlashAndNfc() {
    String nfd = Normalizer.normalize("cafe\u0301", Normalizer.Form.NFD);
    String got = PathUtil.normalize("dir\\" + nfd);
    assertEquals("dir/café", got);
    assertEquals(Normalizer.Form.NFC, Normalizer.normalize(got, Normalizer.Form.NFC).equals(got) ? Normalizer.Form.NFC : Normalizer.Form.NFD);
  }

  @Test
  void rejectsDotDot() {
    assertThrows(IllegalArgumentException.class, () -> PathUtil.normalize("foo/../secret"));
    assertThrows(IllegalArgumentException.class, () -> PathUtil.normalize("..\\windows"));
  }
}
