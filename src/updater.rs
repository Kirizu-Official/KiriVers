use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use crate::adapters::{check_capabilities, Adapters};
use crate::client::Client;
use crate::config::Config;
use crate::error::Error;
use crate::hasher::eq_hex;
use crate::pathutil::normalize_path;
use crate::signature::payload_from_parts;
use crate::types::*;

/// Pack poll backoff. Default 1s → cap 15s, 120s deadline.
#[derive(Debug, Clone)]
pub struct PackPollOptions {
    pub initial_delay: Duration,
    pub max_delay: Duration,
    pub deadline: Duration,
}

impl Default for PackPollOptions {
    fn default() -> Self {
        Self {
            initial_delay: Duration::from_secs(1),
            max_delay: Duration::from_secs(15),
            deadline: Duration::from_secs(120),
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum UpdateStatus {
    NoUpdate,
    NotModified,
    Downloaded,
    Applied,
}

#[derive(Debug, Clone)]
pub struct UpdateResult {
    pub status: UpdateStatus,
    pub check: Option<UpdateCheck>,
    pub etag: Option<String>,
    pub staged_path: Option<PathBuf>,
    pub applied: bool,
    pub diff_mode: Option<String>,
}

#[derive(Debug, Clone)]
pub struct UpdateRequest {
    pub current_version: String,
    pub os: String,
    pub arch: String,
    pub channel: Option<String>,
    pub device_id: Option<String>,
    pub hw_rev: Option<String>,
    pub os_version: Option<String>,
    pub custom: Option<serde_json::Value>,
    /// Report the device to the namebook before check.
    pub report_device: bool,
    pub if_none_match: Option<String>,
    /// Local install root for multi-file integrity compare.
    pub install_dir: Option<PathBuf>,
    /// Current single-file binary used as delta basis.
    pub local_file: Option<PathBuf>,
    /// Directory for verified bytes when not applying, or staging before replace.
    pub stage_dir: PathBuf,
    /// Destination for the default/injected Replacer.
    pub dest: Option<PathBuf>,
    pub pack_poll: PackPollOptions,
}

/// High-level check → download → verify → optional patch/unpack → optional replace.
pub struct Updater {
    client: Client,
    adapters: Adapters,
}

impl Updater {
    pub fn new(client: Client, adapters: Adapters) -> Self {
        Self { client, adapters }
    }

    pub fn with_defaults(config: Config) -> Result<Self, Error> {
        Ok(Self {
            client: Client::new(config)?,
            adapters: Adapters::defaults(),
        })
    }

    pub fn client(&self) -> &Client {
        &self.client
    }

    pub fn adapters(&self) -> &Adapters {
        &self.adapters
    }

    pub fn check_request(&self, req: &UpdateRequest) -> CheckRequest {
        let (capabilities, accepted_delta_algos) = check_capabilities(&self.adapters);
        CheckRequest {
            current_version: req.current_version.clone(),
            os: req.os.clone(),
            arch: req.arch.clone(),
            channel: req.channel.clone(),
            hw_rev: req.hw_rev.clone(),
            os_version: req.os_version.clone(),
            device_id: req.device_id.clone(),
            capabilities,
            accepted_delta_algos,
        }
    }

    pub fn run(&self, req: &UpdateRequest) -> Result<UpdateResult, Error> {
        if req.report_device {
            if let Some(id) = req.device_id.as_ref().filter(|s| !s.is_empty()) {
                let _ = self.client.device_report(&DeviceReportInput {
                    device_id: id.clone(),
                    version: Some(req.current_version.clone()),
                    os: Some(req.os.clone()),
                    arch: Some(req.arch.clone()),
                    channel: req.channel.clone(),
                    custom: req.custom.clone(),
                });
            }
        }

        let check_req = self.check_request(req);
        let outcome = self.client.check_with(
            &check_req,
            &RequestOptions {
                if_none_match: req.if_none_match.clone(),
                ..RequestOptions::default()
            },
        )?;

        match outcome {
            CheckOutcome::NoUpdate { etag } => Ok(UpdateResult {
                status: UpdateStatus::NoUpdate,
                check: None,
                etag,
                staged_path: None,
                applied: false,
                diff_mode: None,
            }),
            CheckOutcome::NotModified { etag } => Ok(UpdateResult {
                status: UpdateStatus::NotModified,
                check: None,
                etag,
                staged_path: None,
                applied: false,
                diff_mode: None,
            }),
            CheckOutcome::Update { body, etag } => match self.apply_update(req, &body) {
                Ok(mut result) => {
                    result.etag = etag;
                    result.check = Some(body);
                    let _ = self.telemetry(
                        req,
                        result.check.as_ref(),
                        "installed",
                        result.diff_mode.as_deref(),
                        None,
                    );
                    Ok(result)
                }
                Err(e) => {
                    let _ = self.telemetry(req, Some(&body), "failed", None, Some(&e));
                    Err(e)
                }
            },
        }
    }

    fn apply_update(
        &self,
        req: &UpdateRequest,
        check: &UpdateCheck,
    ) -> Result<UpdateResult, Error> {
        self.verify_signature(check)?;

        std::fs::create_dir_all(&req.stage_dir)?;

        let hasher = self
            .adapters
            .hasher
            .clone()
            .unwrap_or_else(|| Arc::new(crate::hasher::StdHasher));

        let (bytes, diff_mode, name_hint) = self.fetch_bytes(req, check, hasher.as_ref())?;
        if diff_mode == "full_package" {
            let actual = hasher.sha256_hex(&bytes);
            if !eq_hex(&check.sha256, &actual) {
                return Err(Error::HashMismatch {
                    expected: check.sha256.clone(),
                    actual,
                });
            }
        }

        let file_name = if name_hint.is_empty() {
            if check.file_name.is_empty() {
                format!("{}.bin", check.sha256)
            } else {
                check.file_name.clone()
            }
        } else {
            name_hint
        };
        let staged = req.stage_dir.join(&file_name);
        std::fs::write(&staged, &bytes)?;

        let mut applied = false;
        if let (Some(replacer), Some(dest)) = (&self.adapters.replacer, req.dest.as_ref()) {
            replacer.replace(&staged, dest)?;
            applied = true;
        }

        Ok(UpdateResult {
            status: if applied {
                UpdateStatus::Applied
            } else {
                UpdateStatus::Downloaded
            },
            check: None,
            etag: None,
            staged_path: Some(if applied {
                req.dest.clone().unwrap_or(staged)
            } else {
                staged
            }),
            applied,
            diff_mode: Some(diff_mode),
        })
    }

    fn fetch_bytes(
        &self,
        req: &UpdateRequest,
        check: &UpdateCheck,
        hasher: &dyn crate::hasher::Hasher,
    ) -> Result<(Vec<u8>, String, String), Error> {
        let single = check.package_type == "single_file";
        let want_delta = single
            && self.adapters.patcher.is_some()
            && check.delta_available
            && !check.is_downgrade;

        if want_delta {
            match self.try_delta(req, check, hasher) {
                Ok(v) => return Ok(v),
                Err(_) => {
                    // Unknown magic, hash mismatch, or missing local basis → full package.
                }
            }
        }

        if check.package_type == "multi_file" && self.adapters.file_store.is_some() {
            if let Ok(v) = self.try_pack(req, check, hasher) {
                return Ok(v);
            }
        }

        let dl = self.client.download(&check.package_url, None)?;
        let actual = hasher.sha256_hex(&dl.body);
        if !eq_hex(&check.sha256, &actual) {
            return Err(Error::HashMismatch {
                expected: check.sha256.clone(),
                actual,
            });
        }
        Ok((dl.body, "full_package".into(), check.file_name.clone()))
    }

    fn try_delta(
        &self,
        req: &UpdateRequest,
        check: &UpdateCheck,
        hasher: &dyn crate::hasher::Hasher,
    ) -> Result<(Vec<u8>, String, String), Error> {
        let patcher = self
            .adapters
            .patcher
            .as_ref()
            .ok_or_else(|| Error::Config("patcher required for binary_delta".into()))?;
        let local_path = req
            .local_file
            .as_ref()
            .ok_or_else(|| Error::Config("local_file required for binary_delta".into()))?;
        let basis = std::fs::read(local_path)?;
        let local_sha = hasher.sha256_hex(&basis);
        let (caps, algos) = check_capabilities(&self.adapters);
        let diff = self.client.diff(&DiffRequest {
            source_version: req.current_version.clone(),
            target_version: check.version_semver.clone().unwrap_or_else(|| {
                check
                    .version_integer
                    .map(|n| n.to_string())
                    .unwrap_or_default()
            }),
            os: req.os.clone(),
            arch: req.arch.clone(),
            channel: req.channel.clone(),
            device_id: req.device_id.clone(),
            hw_rev: req.hw_rev.clone(),
            local_sha256: Some(local_sha),
            capabilities: caps,
            accepted_delta_algos: algos,
            prefer_full: Some(false),
        })?;
        if diff.diff_mode != "binary_delta" {
            return Err(Error::Config(format!("diff_mode {}", diff.diff_mode)));
        }
        let url = diff
            .package_url
            .as_deref()
            .ok_or_else(|| Error::Config("diff missing package_url".into()))?;
        let dl = self.client.download(url, None)?;
        if let Some(exp) = diff.sha256.as_deref() {
            let actual = hasher.sha256_hex(&dl.body);
            if !eq_hex(exp, &actual) {
                return Err(Error::HashMismatch {
                    expected: exp.to_string(),
                    actual,
                });
            }
        }
        let algo = diff
            .delta_algo
            .as_deref()
            .or(check.delta_algo.as_deref())
            .unwrap_or("");
        crate::patcher::assert_magic_matches(algo, &dl.body)?;
        let patched = patcher.apply(algo, &basis, &dl.body)?;
        let actual = hasher.sha256_hex(&patched);
        if !eq_hex(&check.sha256, &actual) {
            return Err(Error::HashMismatch {
                expected: check.sha256.clone(),
                actual,
            });
        }
        Ok((patched, "binary_delta".into(), check.file_name.clone()))
    }

    fn try_pack(
        &self,
        req: &UpdateRequest,
        check: &UpdateCheck,
        hasher: &dyn crate::hasher::Hasher,
    ) -> Result<(Vec<u8>, String, String), Error> {
        let store = self.adapters.file_store.as_ref().unwrap();
        let install = req
            .install_dir
            .as_ref()
            .ok_or_else(|| Error::Config("install_dir required for pack".into()))?;
        let target = check.version_semver.clone().unwrap_or_else(|| {
            check
                .version_integer
                .map(|n| n.to_string())
                .unwrap_or_default()
        });
        let integ = match self.client.integrity(&IntegrityQuery {
            version: target.clone(),
            os: req.os.clone(),
            arch: req.arch.clone(),
            channel: req.channel.clone(),
            hw_rev: req.hw_rev.clone(),
            ..IntegrityQuery::default()
        })? {
            Cached::Fresh { value, .. } => value,
            Cached::NotModified { .. } => {
                return Err(Error::Config("integrity 304 without body".into()));
            }
        };
        let needed = needed_paths(store.as_ref(), hasher, install, &integ.files)?;
        let pack_req = PackRequest {
            source_version: req.current_version.clone(),
            target_version: target,
            os: req.os.clone(),
            arch: req.arch.clone(),
            needed_paths: needed,
            channel: req.channel.clone(),
            device_id: req.device_id.clone(),
            hw_rev: req.hw_rev.clone(),
        };
        let pack = self
            .client
            .pack_until_ready(&pack_req, req.pack_poll.clone())?;
        if pack.status == "full_package" {
            let dl = self.client.download(&check.package_url, None)?;
            let actual = hasher.sha256_hex(&dl.body);
            if !eq_hex(&check.sha256, &actual) {
                return Err(Error::HashMismatch {
                    expected: check.sha256.clone(),
                    actual,
                });
            }
            return Ok((dl.body, "full_package".into(), check.file_name.clone()));
        }
        if pack.status != "ready" {
            return Err(Error::Config(format!(
                "unexpected pack status {}",
                pack.status
            )));
        }
        let url = pack
            .package_url
            .as_deref()
            .ok_or_else(|| Error::Config("pack ready missing package_url".into()))?;
        let dl = self.client.download(url, None)?;
        if let Some(exp) = pack.sha256.as_deref() {
            let actual = hasher.sha256_hex(&dl.body);
            if !eq_hex(exp, &actual) {
                return Err(Error::HashMismatch {
                    expected: exp.to_string(),
                    actual,
                });
            }
        }
        if let Some(unpacker) = &self.adapters.unpacker {
            if let Some(files) = &pack.files {
                unpacker.unpack(&dl.body, files, &req.stage_dir)?;
            }
        }
        Ok((
            dl.body,
            "patch_package".into(),
            pack.file_name.clone().unwrap_or_default(),
        ))
    }

    fn verify_signature(&self, check: &UpdateCheck) -> Result<(), Error> {
        let Some(sig) = check.signature.as_deref().filter(|s| !s.is_empty()) else {
            return Ok(());
        };
        if self.client.config().verify_keys.is_empty() {
            return Ok(());
        }
        let verifier = self
            .adapters
            .signatures
            .clone()
            .unwrap_or_else(|| Arc::new(crate::signature::StdSignatureVerifier));
        let payload = payload_from_parts(
            check.version_integer,
            check.version_semver.as_deref(),
            &check.root_hash,
            &check.package_url,
            check.size,
            &check.sha256,
        );
        let mut last = Error::Signature("no keys configured".into());
        for key in &self.client.config().verify_keys {
            match verifier.verify(&key.algo, &key.public_key_pem, &payload, sig) {
                Ok(()) => return Ok(()),
                Err(e) => last = e,
            }
        }
        Err(last)
    }

    fn telemetry(
        &self,
        req: &UpdateRequest,
        check: Option<&UpdateCheck>,
        status: &str,
        diff_mode: Option<&str>,
        err: Option<&Error>,
    ) -> Result<(), Error> {
        let Some(check) = check else {
            return Ok(());
        };
        let channel = req
            .channel
            .clone()
            .unwrap_or_else(|| check.target_channel.clone());
        let to = check.version_semver.clone().unwrap_or_else(|| {
            check
                .version_integer
                .map(|n| n.to_string())
                .unwrap_or_default()
        });
        let report = TelemetryReport {
            os: req.os.clone(),
            arch: req.arch.clone(),
            channel,
            from_version: req.current_version.clone(),
            to_version: to,
            status: status.into(),
            device_id: req.device_id.clone(),
            diff_mode: diff_mode.filter(|s| !s.is_empty()).map(ToOwned::to_owned),
            error_code: err.and_then(|e| e.code().map(ToOwned::to_owned)),
            error_message: err.map(|e| e.to_string()),
        };
        self.client.report_telemetry(&report)
    }
}

/// Paths the local tree still needs. KEEP_IF_EXISTS files that exist are omitted.
/// OVERWRITE files whose local SHA-256 already matches are omitted. Invalid paths skipped.
pub fn needed_paths(
    store: &dyn crate::filestore::FileStore,
    hasher: &dyn crate::hasher::Hasher,
    install: &Path,
    files: &[IntegrityFile],
) -> Result<Vec<String>, Error> {
    let mut needed = Vec::new();
    for f in files {
        let Ok(rel) = normalize_path(&f.path) else {
            continue;
        };
        let policy = f
            .install_policy
            .as_deref()
            .unwrap_or("OVERWRITE")
            .to_ascii_uppercase();
        let exists = store.exists(install, &rel).unwrap_or(false);
        if policy == "KEEP_IF_EXISTS" && exists {
            continue;
        }
        if exists {
            if let Ok(bytes) = store.read(install, &rel) {
                if let Some(exp) = f.sha256.as_deref() {
                    if eq_hex(exp, &hasher.sha256_hex(&bytes)) {
                        continue;
                    }
                }
            }
        }
        if !needed.iter().any(|e| e == &rel) {
            needed.push(rel);
        }
    }
    Ok(needed)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::filestore::StdFileStore;
    use crate::hasher::{Hasher, StdHasher};

    #[test]
    fn keep_if_exists_omitted() {
        let dir = std::env::temp_dir().join(format!("kirivers-need-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        std::fs::write(dir.join("keep.txt"), b"old").unwrap();
        std::fs::write(dir.join("same.bin"), b"abc").unwrap();
        let h = StdHasher;
        let same_hash = h.sha256_hex(b"abc");
        let files = vec![
            IntegrityFile {
                path: "keep.txt".into(),
                size: 3,
                sha256: Some("dead".into()),
                md5: None,
                url: None,
                install_policy: Some("KEEP_IF_EXISTS".into()),
                integrity_check: Some(false),
            },
            IntegrityFile {
                path: "same.bin".into(),
                size: 3,
                sha256: Some(same_hash),
                md5: None,
                url: None,
                install_policy: Some("OVERWRITE".into()),
                integrity_check: Some(true),
            },
            IntegrityFile {
                path: "missing.bin".into(),
                size: 1,
                sha256: Some("aa".into()),
                md5: None,
                url: None,
                install_policy: Some("OVERWRITE".into()),
                integrity_check: Some(true),
            },
        ];
        let n = needed_paths(&StdFileStore, &h, &dir, &files).unwrap();
        assert_eq!(n, vec!["missing.bin"]);
        let _ = std::fs::remove_dir_all(&dir);
    }
}
