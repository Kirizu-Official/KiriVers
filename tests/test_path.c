#include "kirivers/path.h"
#include "test.h"

int main(void) {
    char *out = NULL;
    KiriversError err;
    memset(&err, 0, sizeof(err));

    EXPECT(kirivers_path_normalize("foo\\bar", &out, &err) == KIRIVERS_OK);
    EXPECT_STREQ(out, "foo/bar");
    free(out);

    EXPECT(kirivers_path_normalize("a/../b", &out, &err) == KIRIVERS_ERR);
    EXPECT_STREQ(err.code, "INVALID_PATH");
    kirivers_error_clear(&err);

    EXPECT(kirivers_path_normalize("/abs", &out, &err) == KIRIVERS_ERR);
    kirivers_error_clear(&err);

    EXPECT(kirivers_path_normalize("C:windows", &out, &err) == KIRIVERS_ERR);
    kirivers_error_clear(&err);

    EXPECT(kirivers_path_normalize("ok//path", &out, &err) == KIRIVERS_OK);
    EXPECT_STREQ(out, "ok/path");
    free(out);

#ifdef KIRIVERS_HOSTED
    {
        /* NFC: e + combining acute → U+00E9 */
        const char nfd[] = {'c', 'a', 'f', 'e', (char)0xCC, (char)0x81, 0};
        EXPECT(kirivers_path_normalize(nfd, &out, &err) == KIRIVERS_OK);
        EXPECT_STREQ(out, "caf\xc3\xa9");
        free(out);
    }
#endif

    printf("test_path ok\n");
    return 0;
}
