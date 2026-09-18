use std::sync::Arc;

use crate::filestore::{FileStore, StdFileStore};
use crate::hasher::{Hasher, StdHasher};
use crate::patcher::Patcher;
use crate::replacer::{RenameReplacer, Replacer};
use crate::signature::{SignatureVerifier, StdSignatureVerifier};
use crate::unpacker::{ArchiveUnpacker, ZipUnpacker};

/// Optional updater adapters. Defaults follow parent D18; `Patcher` has no default.
#[derive(Clone, Default)]
pub struct Adapters {
    pub file_store: Option<Arc<dyn FileStore>>,
    pub hasher: Option<Arc<dyn Hasher>>,
    pub patcher: Option<Arc<dyn Patcher>>,
    pub replacer: Option<Arc<dyn Replacer>>,
    pub unpacker: Option<Arc<dyn ArchiveUnpacker>>,
    pub signatures: Option<Arc<dyn SignatureVerifier>>,
}

impl Adapters {
    /// FileStore + Hasher + zip ArchiveUnpacker + rename Replacer + signature verifier. No Patcher.
    pub fn defaults() -> Self {
        Self {
            file_store: Some(Arc::new(StdFileStore)),
            hasher: Some(Arc::new(StdHasher)),
            patcher: None,
            replacer: Some(Arc::new(RenameReplacer)),
            unpacker: Some(Arc::new(ZipUnpacker)),
            signatures: Some(Arc::new(StdSignatureVerifier)),
        }
    }
}

/// D13 capability bits derived from live adapters.
///
/// Default check without adapters is `["full_package"]` only. A default [`Adapters::defaults`]
/// set also sends `patch_package` (zip) and `file_list` (FileStore). `binary_delta` is sent
/// only when a `Patcher` advertises at least one algorithm.
pub fn check_capabilities(adapters: &Adapters) -> (Vec<String>, Vec<String>) {
    let mut caps = vec!["full_package".to_string()];
    let mut algos = Vec::new();
    if let Some(p) = &adapters.patcher {
        algos = p
            .supported_algos()
            .into_iter()
            .filter(|a| !a.is_empty())
            .collect();
        if !algos.is_empty() {
            caps.push("binary_delta".to_string());
        }
    }
    if adapters.unpacker.is_some() {
        caps.push("patch_package".to_string());
    }
    if adapters
        .file_store
        .as_ref()
        .map(|f| f.supports_file_list())
        .unwrap_or(false)
    {
        caps.push("file_list".to_string());
    }
    (caps, algos)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::error::Error;
    use crate::patcher::Patcher;

    struct FakePatcher;

    impl Patcher for FakePatcher {
        fn supported_algos(&self) -> Vec<String> {
            vec!["bsdiff".into(), "hdiffpatch".into()]
        }
        fn apply(&self, _algo: &str, _basis: &[u8], _delta: &[u8]) -> Result<Vec<u8>, Error> {
            Err(Error::UnknownDeltaMagic)
        }
    }

    #[test]
    fn default_has_no_binary_delta() {
        let (caps, algos) = check_capabilities(&Adapters::defaults());
        assert!(caps.contains(&"full_package".into()));
        assert!(caps.contains(&"patch_package".into()));
        assert!(caps.contains(&"file_list".into()));
        assert!(!caps.contains(&"binary_delta".into()));
        assert!(algos.is_empty());
    }

    #[test]
    fn empty_adapters_only_full_package() {
        let (caps, algos) = check_capabilities(&Adapters::default());
        assert_eq!(caps, vec!["full_package"]);
        assert!(algos.is_empty());
    }

    #[test]
    fn injected_patcher_declares_algos() {
        let mut a = Adapters::defaults();
        a.patcher = Some(Arc::new(FakePatcher));
        let (caps, algos) = check_capabilities(&a);
        assert!(caps.contains(&"binary_delta".into()));
        assert_eq!(algos, vec!["bsdiff", "hdiffpatch"]);
    }
}
