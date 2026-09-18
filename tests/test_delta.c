#include "kirivers/adapters.h"
#include "test.h"

int main(void) {
    const uint8_t hp[] = "HDIFF13&xxxx";
    const uint8_t kv[] = "KVDIFFHP1\nxxxx";
    const uint8_t bs[] = "BSDIFF40xxxx";
    const uint8_t vcd[] = {0xD6, 0xC3, 0xC4, 0x00};
    const uint8_t unk[] = "NOTAMAGIC";
    EXPECT_STREQ(kirivers_delta_algo_from_magic(hp, sizeof(hp) - 1), "hdiffpatch");
    EXPECT_STREQ(kirivers_delta_algo_from_magic(kv, sizeof(kv) - 1), "hdiffpatch");
    EXPECT_STREQ(kirivers_delta_algo_from_magic(bs, sizeof(bs) - 1), "bsdiff");
    EXPECT_STREQ(kirivers_delta_algo_from_magic(vcd, 4), "xdelta3");
    EXPECT(kirivers_delta_algo_from_magic(unk, sizeof(unk) - 1) == NULL);
    {
        char *p = kirivers_build_check_payload("1", "1.1.0", "rh", "http://u", "10", "ab");
        EXPECT_STREQ(p, "1\n1.1.0\nrh\nhttp://u\n10\nab");
        free(p);
    }
    printf("test_delta ok\n");
    return 0;
}
