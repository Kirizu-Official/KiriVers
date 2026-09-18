#include "kirivers/hosted.h"
#include "internal.h"

#include <string.h>

int kirivers_hosted_adapters_init(KiriversAdapters *out, const char *file_root, KiriversError *err) {
    memset(out, 0, sizeof(*out));
    if (kirivers_transport_curl_init(&out->transport, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    if (kirivers_hasher_openssl_init(&out->hasher, err) != KIRIVERS_OK) {
        kirivers_transport_curl_deinit(&out->transport);
        return KIRIVERS_ERR;
    }
    if (kirivers_unpacker_libzip_init(&out->unpacker, err) != KIRIVERS_OK) {
        kirivers_hasher_openssl_deinit(&out->hasher);
        kirivers_transport_curl_deinit(&out->transport);
        return KIRIVERS_ERR;
    }
    if (file_root && file_root[0]) {
        if (kirivers_filestore_stdio_init(&out->file_store, file_root, err) != KIRIVERS_OK) {
            kirivers_unpacker_libzip_deinit(&out->unpacker);
            kirivers_hasher_openssl_deinit(&out->hasher);
            kirivers_transport_curl_deinit(&out->transport);
            return KIRIVERS_ERR;
        }
    }
    return KIRIVERS_OK;
}

void kirivers_hosted_adapters_deinit(KiriversAdapters *a) {
    if (!a) {
        return;
    }
    kirivers_filestore_stdio_deinit(&a->file_store);
    kirivers_unpacker_libzip_deinit(&a->unpacker);
    kirivers_hasher_openssl_deinit(&a->hasher);
    kirivers_verifier_openssl_deinit(&a->verifier);
    kirivers_transport_curl_deinit(&a->transport);
    memset(a, 0, sizeof(*a));
}
