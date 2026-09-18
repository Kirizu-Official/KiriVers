#ifndef KIRIVERS_TEST_H
#define KIRIVERS_TEST_H

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define EXPECT(cond)                                                                             \
    do {                                                                                         \
        if (!(cond)) {                                                                           \
            fprintf(stderr, "FAIL %s:%d: %s\n", __FILE__, __LINE__, #cond);                      \
            return 1;                                                                            \
        }                                                                                        \
    } while (0)

#define EXPECT_STREQ(a, b)                                                                       \
    do {                                                                                         \
        const char *_a = (a), *_b = (b);                                                         \
        if (!_a || !_b || strcmp(_a, _b) != 0) {                                                 \
            fprintf(stderr, "FAIL %s:%d: \"%s\" != \"%s\"\n", __FILE__, __LINE__,                \
                    _a ? _a : "(null)", _b ? _b : "(null)");                                     \
            return 1;                                                                            \
        }                                                                                        \
    } while (0)

#define EXPECT_CONTAINS(hay, needle)                                                             \
    do {                                                                                         \
        const char *_h = (hay), *_n = (needle);                                                  \
        if (!_h || !_n || !strstr(_h, _n)) {                                                     \
            fprintf(stderr, "FAIL %s:%d: \"%s\" does not contain \"%s\"\n", __FILE__, __LINE__,   \
                    _h ? _h : "(null)", _n ? _n : "(null)");                                     \
            return 1;                                                                            \
        }                                                                                        \
    } while (0)

#endif
