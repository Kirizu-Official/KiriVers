#ifndef KIRIVERS_MOCK_TRANSPORT_H
#define KIRIVERS_MOCK_TRANSPORT_H

#include "kirivers/adapters.h"

#define KIRIVERS_MOCK_MAX_CALLS 24
#define KIRIVERS_MOCK_MAX_SCRIPTS 24

typedef struct KiriversMockCall {
    char method[8];
    char url[512];
    char body[4096];
    char headers[1024];
} KiriversMockCall;

typedef struct KiriversMockScript {
    int status;
    const char *body;
    const char *etag;
    const uint8_t *bin;
    size_t bin_len;
} KiriversMockScript;

typedef struct KiriversMockTransport {
    KiriversMockCall calls[KIRIVERS_MOCK_MAX_CALLS];
    size_t ncalls;
    KiriversMockScript scripts[KIRIVERS_MOCK_MAX_SCRIPTS];
    size_t nscripts;
    size_t script_i;
    int leftover_hit;
} KiriversMockTransport;

void kirivers_mock_init(KiriversMockTransport *m);
void kirivers_mock_add(KiriversMockTransport *m, int status, const char *body);
void kirivers_mock_add_bin(KiriversMockTransport *m, int status, const uint8_t *data, size_t n);
void kirivers_mock_bind(KiriversMockTransport *m, KiriversTransport *t);
int kirivers_mock_leftover(const KiriversMockTransport *m);

#endif
