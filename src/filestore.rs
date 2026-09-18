use std::fs;
use std::path::{Path, PathBuf};

use crate::error::Error;
use crate::pathutil::normalize_path;

/// Read/write/list install trees. Paths are NFC + `/` for fileset compare.
pub trait FileStore: Send + Sync {
    fn read(&self, root: &Path, rel: &str) -> Result<Vec<u8>, Error>;
    fn write(&self, root: &Path, rel: &str, data: &[u8]) -> Result<(), Error>;
    fn exists(&self, root: &Path, rel: &str) -> Result<bool, Error>;
    fn list(&self, root: &Path) -> Result<Vec<String>, Error>;
    /// When true, check may declare `file_list`.
    fn supports_file_list(&self) -> bool {
        true
    }
}

/// Default FileStore: `std::fs` plus NFC path rules.
#[derive(Debug, Default, Clone, Copy)]
pub struct StdFileStore;

impl StdFileStore {
    fn native_path(root: &Path, rel: &str) -> Result<PathBuf, Error> {
        let n = normalize_path(rel)?;
        let mut p = root.to_path_buf();
        for seg in n.split('/') {
            p.push(seg);
        }
        Ok(p)
    }
}

impl FileStore for StdFileStore {
    fn read(&self, root: &Path, rel: &str) -> Result<Vec<u8>, Error> {
        Ok(fs::read(Self::native_path(root, rel)?)?)
    }

    fn write(&self, root: &Path, rel: &str, data: &[u8]) -> Result<(), Error> {
        let dest = Self::native_path(root, rel)?;
        if let Some(parent) = dest.parent() {
            fs::create_dir_all(parent)?;
        }
        fs::write(dest, data)?;
        Ok(())
    }

    fn exists(&self, root: &Path, rel: &str) -> Result<bool, Error> {
        Ok(Self::native_path(root, rel)?.exists())
    }

    fn list(&self, root: &Path) -> Result<Vec<String>, Error> {
        let mut out = Vec::new();
        walk(root, root, &mut out)?;
        Ok(out)
    }
}

fn walk(root: &Path, dir: &Path, out: &mut Vec<String>) -> Result<(), Error> {
    if !dir.exists() {
        return Ok(());
    }
    for entry in fs::read_dir(dir)? {
        let entry = entry?;
        let path = entry.path();
        if path.is_dir() {
            walk(root, &path, out)?;
        } else if let Ok(rel) = path.strip_prefix(root) {
            let joined = rel
                .components()
                .map(|c| c.as_os_str().to_string_lossy().into_owned())
                .collect::<Vec<_>>()
                .join("/");
            if let Ok(n) = normalize_path(&joined) {
                out.push(n);
            }
        }
    }
    Ok(())
}
