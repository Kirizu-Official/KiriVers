#include "kirivers/updater.h"
#include "internal.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

struct KiriversUpdater {
    KiriversClient *client;
    KiriversAdapters adapters;
    char *signature_algo;
};

KiriversUpdater *kirivers_updater_new(const KiriversUpdaterConfig *cfg, KiriversError *err) {
    KiriversUpdater *u;
    if (!cfg || !cfg->client) {
        kirivers_error_set(err, 0, "INVALID_ARGUMENT", "client is required", NULL);
        return NULL;
    }
    u = (KiriversUpdater *)calloc(1, sizeof(*u));
    if (!u) {
        kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
        return NULL;
    }
    u->client = cfg->client;
    u->adapters = cfg->adapters;
    u->signature_algo = cfg->signature_algo ? kirivers_strdup(cfg->signature_algo) : kirivers_strdup("ed25519");
    return u;
}

void kirivers_updater_free(KiriversUpdater *u) {
    if (!u) {
        return;
    }
    free(u->signature_algo);
    free(u);
}

void kirivers_update_result_free(KiriversUpdateResult *v) {
    if (!v) {
        return;
    }
    free(v->staged_path);
    free(v->sha256);
    free(v->version_semver);
    free(v->diff_mode);
    kirivers_check_result_free(&v->check);
    memset(v, 0, sizeof(*v));
}

static int hex_eq(const char *a, const char *b) {
    if (!a || !b) {
        return 0;
    }
    while (*a && *b) {
        char ca = *a, cb = *b;
        if (ca >= 'A' && ca <= 'F') {
            ca = (char)(ca - 'A' + 'a');
        }
        if (cb >= 'A' && cb <= 'F') {
            cb = (char)(cb - 'A' + 'a');
        }
        if (ca != cb) {
            return 0;
        }
        a++;
        b++;
    }
    return *a == 0 && *b == 0;
}

static int verify_sha(KiriversUpdater *u, const uint8_t *data, size_t n, const char *expect,
                      KiriversError *err) {
    char hex[65];
    if (!u->adapters.hasher.sha256 || !expect || !expect[0]) {
        return KIRIVERS_OK;
    }
    memset(hex, 0, sizeof(hex));
    if (u->adapters.hasher.sha256(u->adapters.hasher.ctx, data, n, hex, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    if (!hex_eq(hex, expect)) {
        return kirivers_error_set(err, 0, "HASH_MISMATCH", "downloaded sha256 did not match", NULL);
    }
    return KIRIVERS_OK;
}

static int verify_sig(KiriversUpdater *u, const KiriversCheckResult *ck, KiriversError *err) {
    char *payload, isize[32];
    int rc;
    if (!ck->signature || !ck->signature[0] || !u->adapters.verifier.verify) {
        return KIRIVERS_OK;
    }
    if (ck->has_version_integer) {
        snprintf(isize, sizeof(isize), "%lld", (long long)ck->version_integer);
    } else {
        isize[0] = 0;
    }
    {
        char sizebuf[32];
        snprintf(sizebuf, sizeof(sizebuf), "%lld", (long long)ck->size);
        payload = kirivers_build_check_payload(ck->has_version_integer ? isize : "",
                                               ck->version_semver ? ck->version_semver : "",
                                               ck->root_hash ? ck->root_hash : "",
                                               ck->package_url ? ck->package_url : "", sizebuf,
                                               ck->sha256 ? ck->sha256 : "");
    }
    if (!payload) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = u->adapters.verifier.verify(u->adapters.verifier.ctx, u->signature_algo, payload,
                                     ck->signature, err);
    free(payload);
    return rc;
}

static int store_write(KiriversUpdater *u, const char *rel, const uint8_t *data, size_t n,
                       KiriversError *err) {
    if (!u->adapters.file_store.write_all) {
        return kirivers_error_set(err, 0, "NO_FILESTORE", "FileStore.write_all required to stage",
                                  NULL);
    }
    return u->adapters.file_store.write_all(u->adapters.file_store.ctx, rel, data, n, err);
}

static int magic_matches_patcher(KiriversUpdater *u, const uint8_t *delta, size_t n,
                                 KiriversError *err) {
    const char *got = kirivers_delta_algo_from_magic(delta, n);
    const char *const *algos = NULL;
    size_t count = 0, i;
    if (!got) {
        return kirivers_error_set(err, 0, "DELTA_MAGIC", "unknown delta magic", NULL);
    }
    if (!u->adapters.patcher.supported_algos) {
        return kirivers_error_set(err, 0, "DELTA_MAGIC", "patcher does not advertise algos", NULL);
    }
    if (u->adapters.patcher.supported_algos(u->adapters.patcher.ctx, &algos, &count) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    for (i = 0; i < count; i++) {
        if (algos[i] && strcmp(algos[i], got) == 0) {
            return KIRIVERS_OK;
        }
    }
    return kirivers_error_set(err, 0, "DELTA_MAGIC", "delta magic does not match advertised algo",
                              NULL);
}

static void telemetry_quiet(KiriversUpdater *u, const KiriversUpdateRequest *req,
                            const KiriversCheckResult *ck, const char *status, const char *diff_mode,
                            const KiriversError *fail) {
    KiriversTelemetryRequest tr;
    KiriversError ignore;
    memset(&tr, 0, sizeof(tr));
    memset(&ignore, 0, sizeof(ignore));
    tr.os = req->os;
    tr.arch = req->arch;
    tr.channel = req->channel ? req->channel : "stable";
    tr.from_version = req->current_version;
    tr.to_version = ck && ck->version_semver ? ck->version_semver : req->current_version;
    tr.status = status;
    tr.device_id = req->device_id;
    tr.diff_mode = diff_mode;
    if (fail && fail->code) {
        tr.error_code = fail->code;
        tr.error_message = fail->message;
    }
    (void)kirivers_client_telemetry(u->client, &tr, &ignore);
    kirivers_error_clear(&ignore);
}

static int try_binary_delta(KiriversUpdater *u, const KiriversUpdateRequest *req,
                            const KiriversCheckResult *ck, KiriversBytes *out, char **mode,
                            KiriversError *err) {
    KiriversDiffRequest dreq;
    KiriversDiffResult diff;
    KiriversCapabilitySet caps;
    KiriversBytes delta;
    uint8_t *oldb = NULL, *newb = NULL;
    size_t oldn = 0, newn = 0;
    int rc;

    if (!u->adapters.patcher.apply || !ck->delta_available || ck->is_downgrade ||
        !req->local_sha256) {
        return KIRIVERS_ERR;
    }
    memset(&caps, 0, sizeof(caps));
    if (kirivers_derive_capabilities(&u->adapters, &caps, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    memset(&dreq, 0, sizeof(dreq));
    dreq.source_version = req->current_version;
    dreq.target_version = ck->version_semver;
    dreq.os = req->os;
    dreq.arch = req->arch;
    dreq.channel = ck->target_channel;
    dreq.hw_rev = req->hw_rev;
    dreq.device_id = req->device_id;
    dreq.local_sha256 = req->local_sha256;
    dreq.capabilities = (const char *const *)caps.capabilities;
    dreq.n_capabilities = caps.n_capabilities;
    dreq.accepted_delta_algos = (const char *const *)caps.accepted_delta_algos;
    dreq.n_accepted_delta_algos = caps.n_algos;
    memset(&diff, 0, sizeof(diff));
    rc = kirivers_client_diff(u->client, &dreq, &diff, err);
    kirivers_capability_set_free(&caps);
    if (rc != KIRIVERS_OK || !diff.diff_mode || strcmp(diff.diff_mode, "binary_delta") != 0 ||
        !diff.package_url) {
        kirivers_diff_result_free(&diff);
        return KIRIVERS_ERR;
    }
    memset(&delta, 0, sizeof(delta));
    rc = kirivers_client_download_url(u->client, diff.package_url, NULL, &delta, err);
    if (rc != KIRIVERS_OK) {
        kirivers_diff_result_free(&diff);
        return KIRIVERS_ERR;
    }
    if (magic_matches_patcher(u, delta.data, delta.len, err) != KIRIVERS_OK) {
        kirivers_bytes_free(&delta);
        kirivers_diff_result_free(&diff);
        return KIRIVERS_ERR;
    }
    if (u->adapters.file_store.read_all && req->install_path) {
        if (u->adapters.file_store.read_all(u->adapters.file_store.ctx, req->install_path, &oldb,
                                            &oldn, err) != KIRIVERS_OK) {
            kirivers_bytes_free(&delta);
            kirivers_diff_result_free(&diff);
            return KIRIVERS_ERR;
        }
    }
    rc = u->adapters.patcher.apply(u->adapters.patcher.ctx, oldb, oldn, delta.data, delta.len, &newb,
                                   &newn, err);
    free(oldb);
    kirivers_bytes_free(&delta);
    if (rc != KIRIVERS_OK || !newb) {
        kirivers_diff_result_free(&diff);
        free(newb);
        return KIRIVERS_ERR;
    }
    if (verify_sha(u, newb, newn, ck->sha256, err) != KIRIVERS_OK) {
        kirivers_diff_result_free(&diff);
        free(newb);
        return KIRIVERS_ERR;
    }
    out->data = newb;
    out->len = newn;
    *mode = kirivers_strdup("binary_delta");
    kirivers_diff_result_free(&diff);
    return KIRIVERS_OK;
}

static int local_hash(KiriversUpdater *u, const char *rel, char hex[65], KiriversError *err) {
    uint8_t *data = NULL;
    size_t n = 0;
    int rc;
    hex[0] = 0;
    if (!u->adapters.file_store.read_all || !u->adapters.hasher.sha256) {
        return KIRIVERS_ERR;
    }
    if (u->adapters.file_store.read_all(u->adapters.file_store.ctx, rel, &data, &n, err) !=
        KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    rc = u->adapters.hasher.sha256(u->adapters.hasher.ctx, data, n, hex, err);
    free(data);
    return rc;
}

static int try_pack(KiriversUpdater *u, const KiriversUpdateRequest *req, const KiriversCheckResult *ck,
                    KiriversBytes *out, char **mode, KiriversError *err) {
    KiriversIntegrityQuery iq;
    KiriversIntegrity integ;
    KiriversPackRequest preq;
    KiriversPackResult pack;
    char **needed = NULL;
    size_t n_needed = 0, i;
    int rc;

    if (!ck->package_type || strcmp(ck->package_type, "multi_file") != 0 || !req->install_dir ||
        !u->adapters.file_store.read_all || !u->adapters.hasher.sha256) {
        return KIRIVERS_ERR;
    }
    memset(&iq, 0, sizeof(iq));
    iq.version = ck->version_semver;
    iq.os = req->os;
    iq.arch = req->arch;
    iq.channel = ck->target_channel;
    iq.hw_rev = req->hw_rev;
    iq.compact = -1;
    iq.include_file_urls = -1;
    memset(&integ, 0, sizeof(integ));
    rc = kirivers_client_integrity(u->client, &iq, &integ, err);
    if (rc != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    for (i = 0; i < integ.n_files; i++) {
        KiriversIntegrityFile *f = &integ.files[i];
        char *norm = NULL;
        int exists = 0;
        const char *rel = f->path;
        if (u->adapters.file_store.normalize) {
            if (u->adapters.file_store.normalize(u->adapters.file_store.ctx, f->path, &norm, err) !=
                KIRIVERS_OK) {
                continue;
            }
            rel = norm;
        }
        if (f->install_policy && strcmp(f->install_policy, "KEEP_IF_EXISTS") == 0) {
            if (u->adapters.file_store.exists) {
                u->adapters.file_store.exists(u->adapters.file_store.ctx, rel, &exists, err);
            }
            if (exists) {
                free(norm);
                continue;
            }
        }
        if (f->integrity_check && f->sha256) {
            char hex[65];
            KiriversError ign;
            memset(&ign, 0, sizeof(ign));
            if (local_hash(u, rel, hex, &ign) == KIRIVERS_OK && hex_eq(hex, f->sha256)) {
                kirivers_error_clear(&ign);
                free(norm);
                continue;
            }
            kirivers_error_clear(&ign);
        }
        {
            char **nx = (char **)realloc(needed, (n_needed + 1) * sizeof(char *));
            if (!nx) {
                free(norm);
                kirivers_free_string_array(needed, n_needed);
                kirivers_integrity_free(&integ);
                return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
            }
            needed = nx;
            needed[n_needed++] = kirivers_strdup(rel);
        }
        free(norm);
    }
    memset(&preq, 0, sizeof(preq));
    preq.source_version = req->current_version;
    preq.target_version = ck->version_semver;
    preq.os = req->os;
    preq.arch = req->arch;
    preq.channel = ck->target_channel;
    preq.hw_rev = req->hw_rev;
    preq.device_id = req->device_id;
    preq.needed_paths = (const char *const *)needed;
    preq.n_needed_paths = n_needed;
    preq.poll = 1;
    memset(&pack, 0, sizeof(pack));
    rc = kirivers_client_pack(u->client, &preq, &pack, err);
    kirivers_free_string_array(needed, n_needed);
    if (rc != KIRIVERS_OK) {
        kirivers_integrity_free(&integ);
        kirivers_pack_result_free(&pack);
        return KIRIVERS_ERR;
    }
    if (pack.status && strcmp(pack.status, "full_package") == 0) {
        kirivers_integrity_free(&integ);
        kirivers_pack_result_free(&pack);
        return KIRIVERS_ERR; /* caller downloads check package_url */
    }
    if (pack.package_url) {
        rc = kirivers_client_download_url(u->client, pack.package_url, NULL, out, err);
        if (rc == KIRIVERS_OK) {
            if (pack.sha256 && verify_sha(u, out->data, out->len, pack.sha256, err) != KIRIVERS_OK) {
                kirivers_bytes_free(out);
                kirivers_integrity_free(&integ);
                kirivers_pack_result_free(&pack);
                return KIRIVERS_ERR;
            }
            *mode = kirivers_strdup(pack.diff_mode ? pack.diff_mode : "patch_package");
            /* Optional unpack of hash-named members into FileStore. */
            if (u->adapters.unpacker.extract_member && u->adapters.file_store.write_all) {
                for (i = 0; i < pack.n_files; i++) {
                    uint8_t *member = NULL;
                    size_t mlen = 0;
                    if (!pack.files[i].sha256 || !pack.files[i].path) {
                        continue;
                    }
                    if (u->adapters.unpacker.extract_member(u->adapters.unpacker.ctx, out->data,
                                                            out->len, pack.files[i].sha256, &member,
                                                            &mlen, err) != KIRIVERS_OK) {
                        continue;
                    }
                    (void)u->adapters.file_store.write_all(u->adapters.file_store.ctx,
                                                           pack.files[i].path, member, mlen, err);
                    free(member);
                }
            }
        }
        kirivers_integrity_free(&integ);
        kirivers_pack_result_free(&pack);
        return rc;
    }
    kirivers_integrity_free(&integ);
    kirivers_pack_result_free(&pack);
    return KIRIVERS_ERR;
}

int kirivers_update(KiriversUpdater *u, const KiriversUpdateRequest *req, KiriversUpdateResult *out,
                    KiriversError *err) {
    KiriversCapabilitySet caps;
    KiriversCheckRequest creq;
    KiriversBytes blob;
    const char *stage;
    int rc;
    KiriversError local;

    memset(out, 0, sizeof(*out));
    memset(&local, 0, sizeof(local));
    if (!u || !req || !req->current_version || !req->os || !req->arch) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT",
                                  "current_version, os, arch required", NULL);
    }
    if (req->report_device && req->device_id) {
        KiriversDeviceReportRequest dr;
        KiriversGeoReport geo;
        memset(&dr, 0, sizeof(dr));
        memset(&geo, 0, sizeof(geo));
        dr.device_id = req->device_id;
        dr.os = req->os;
        dr.arch = req->arch;
        dr.channel = req->channel;
        dr.version = req->current_version;
        (void)kirivers_client_device_report(u->client, &dr, &geo, &local);
        kirivers_geo_report_free(&geo);
        kirivers_error_clear(&local);
    }

    if (kirivers_derive_capabilities(&u->adapters, &caps, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    memset(&creq, 0, sizeof(creq));
    creq.current_version = req->current_version;
    creq.os = req->os;
    creq.arch = req->arch;
    creq.channel = req->channel;
    creq.hw_rev = req->hw_rev;
    creq.os_version = req->os_version;
    creq.device_id = req->device_id;
    creq.if_none_match = req->if_none_match;
    creq.capabilities = (const char *const *)caps.capabilities;
    creq.n_capabilities = caps.n_capabilities;
    creq.accepted_delta_algos = (const char *const *)caps.accepted_delta_algos;
    creq.n_accepted_delta_algos = caps.n_algos;
    rc = kirivers_client_check(u->client, &creq, &out->check, err);
    kirivers_capability_set_free(&caps);
    if (rc == KIRIVERS_NO_UPDATE) {
        out->outcome = KIRIVERS_UPDATE_NO_UPDATE;
        return KIRIVERS_OK;
    }
    if (rc == KIRIVERS_NOT_MODIFIED) {
        out->outcome = KIRIVERS_UPDATE_NOT_MODIFIED;
        return KIRIVERS_OK;
    }
    if (rc != KIRIVERS_OK) {
        return rc;
    }
    if (verify_sig(u, &out->check, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }

    telemetry_quiet(u, req, &out->check, "downloading", "full_package", NULL);
    memset(&blob, 0, sizeof(blob));
    rc = KIRIVERS_ERR;
    kirivers_error_clear(&local);
    if (try_binary_delta(u, req, &out->check, &blob, &out->diff_mode, &local) == KIRIVERS_OK) {
        rc = KIRIVERS_OK;
    } else {
        kirivers_error_clear(&local);
        if (try_pack(u, req, &out->check, &blob, &out->diff_mode, &local) == KIRIVERS_OK) {
            rc = KIRIVERS_OK;
        } else {
            kirivers_error_clear(&local);
            if (!out->check.package_url) {
                return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "check missing package_url",
                                          NULL);
            }
            rc = kirivers_client_download_url(u->client, out->check.package_url, NULL, &blob, err);
            if (rc == KIRIVERS_OK) {
                out->diff_mode = kirivers_strdup("full_package");
            }
        }
    }
    if (rc != KIRIVERS_OK) {
        telemetry_quiet(u, req, &out->check, "failed", out->diff_mode, err);
        return rc;
    }
    /* Full package and patched bytes match check.sha256. A pack zip is a different object
       (already verified against pack.sha256 inside try_pack). */
    if (!out->diff_mode ||
        (strcmp(out->diff_mode, "patch_package") != 0 &&
         strcmp(out->diff_mode, "file_list") != 0)) {
        if (verify_sha(u, blob.data, blob.len, out->check.sha256, err) != KIRIVERS_OK) {
            kirivers_bytes_free(&blob);
            telemetry_quiet(u, req, &out->check, "failed", out->diff_mode, err);
            return KIRIVERS_ERR;
        }
    }

    stage = req->stage_path ? req->stage_path : "kirivers-staged.bin";
    if (store_write(u, stage, blob.data, blob.len, err) != KIRIVERS_OK) {
        /* Still succeed with no FileStore: keep bytes only if write failed for no store.
           R4: download to caller path. If no FileStore, we cannot persist; report staged_path NULL
           but still OK if hasher verified. */
        if (err && err->code && strcmp(err->code, "NO_FILESTORE") == 0) {
            kirivers_error_clear(err);
            out->outcome = KIRIVERS_UPDATE_STAGED;
            out->sha256 = out->check.sha256 ? kirivers_strdup(out->check.sha256) : NULL;
            out->version_semver =
                out->check.version_semver ? kirivers_strdup(out->check.version_semver) : NULL;
            kirivers_bytes_free(&blob);
            telemetry_quiet(u, req, &out->check, "installed", out->diff_mode, NULL);
            return KIRIVERS_OK;
        }
        kirivers_bytes_free(&blob);
        telemetry_quiet(u, req, &out->check, "failed", out->diff_mode, err);
        return KIRIVERS_ERR;
    }
    kirivers_bytes_free(&blob);
    out->staged_path = kirivers_strdup(stage);
    out->sha256 = out->check.sha256 ? kirivers_strdup(out->check.sha256) : NULL;
    out->version_semver = out->check.version_semver ? kirivers_strdup(out->check.version_semver) : NULL;
    out->outcome = KIRIVERS_UPDATE_STAGED;
    if (u->adapters.replacer.replace && req->install_path) {
        telemetry_quiet(u, req, &out->check, "applying", out->diff_mode, NULL);
        if (u->adapters.replacer.replace(u->adapters.replacer.ctx, stage, req->install_path, err) !=
            KIRIVERS_OK) {
            telemetry_quiet(u, req, &out->check, "failed", out->diff_mode, err);
            return KIRIVERS_ERR;
        }
        out->outcome = KIRIVERS_UPDATE_REPLACED;
    }
    telemetry_quiet(u, req, &out->check, "installed", out->diff_mode, NULL);
    return KIRIVERS_OK;
}
