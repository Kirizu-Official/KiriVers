#ifndef KIRIVERS_PATH_H
#define KIRIVERS_PATH_H

#include "kirivers/error.h"

#ifdef __cplusplus
extern "C" {
#endif

/**
 * Normalize a relative fileset path to match the server:
 * reject NUL/control, leading slash, Windows drive letters, `.` / `..` segments;
 * `\\` → `/`; collapse duplicate slashes; trim.
 * Hosted builds also compose Unicode NFC via utf8proc. Embedded builds skip NFC
 * (inject FileStore.normalize if the MCU needs it).
 * On success `*out` is malloc'd.
 */
int kirivers_path_normalize(const char *raw, char **out, KiriversError *err);

#ifdef __cplusplus
}
#endif

#endif
