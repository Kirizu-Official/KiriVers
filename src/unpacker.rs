use std::io::{Cursor, Read};
use std::path::Path;

use crate::error::Error;
use crate::filestore::{FileStore, StdFileStore};
use crate::pathutil::normalize_path;
use crate::types::IntegrityFile;

/// Unpack a native zip. Members are named by content hash; install paths come from `files[].path`.
pub trait ArchiveUnpacker: Send + Sync {
    fn unpack(
        &self,
        archive: &[u8],
        files: &[IntegrityFile],
        dest_root: &Path,
    ) -> Result<(), Error>;
}

/// Default unpacker: the `zip` crate (deflate).
#[derive(Debug, Default, Clone, Copy)]
pub struct ZipUnpacker;

impl ArchiveUnpacker for ZipUnpacker {
    fn unpack(
        &self,
        archive: &[u8],
        files: &[IntegrityFile],
        dest_root: &Path,
    ) -> Result<(), Error> {
        let cursor = Cursor::new(archive);
        let mut zip = zip::ZipArchive::new(cursor).map_err(|e| {
            Error::Io(std::io::Error::new(
                std::io::ErrorKind::InvalidData,
                e.to_string(),
            ))
        })?;
        let store = StdFileStore;
        for file in files {
            let sha = file.sha256.as_deref().unwrap_or("");
            if sha.is_empty() {
                continue;
            }
            let rel = normalize_path(&file.path)?;
            let mut found = None;
            for i in 0..zip.len() {
                let zf = zip.by_index(i).map_err(|e| {
                    Error::Io(std::io::Error::new(
                        std::io::ErrorKind::InvalidData,
                        e.to_string(),
                    ))
                })?;
                if member_matches(zf.name(), sha) {
                    found = Some(i);
                    break;
                }
            }
            let idx = found.ok_or_else(|| {
                Error::Io(std::io::Error::new(
                    std::io::ErrorKind::NotFound,
                    format!("zip member for sha256 {sha} not found"),
                ))
            })?;
            let mut zf = zip.by_index(idx).map_err(|e| {
                Error::Io(std::io::Error::new(
                    std::io::ErrorKind::InvalidData,
                    e.to_string(),
                ))
            })?;
            let mut buf = Vec::new();
            zf.read_to_end(&mut buf)?;
            store.write(dest_root, &rel, &buf)?;
        }
        Ok(())
    }
}

fn member_matches(name: &str, sha256: &str) -> bool {
    let base = name.rsplit('/').next().unwrap_or(name);
    let stem = base.split('.').next().unwrap_or(base);
    stem.eq_ignore_ascii_case(sha256) || base.eq_ignore_ascii_case(sha256)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::hasher::{Hasher, StdHasher};
    use std::io::Write;

    #[test]
    fn unpacks_hash_named_member() {
        let payload = b"hello-zip";
        let sha = StdHasher.sha256_hex(payload);
        let mut cursor = Cursor::new(Vec::new());
        {
            let mut w = zip::ZipWriter::new(&mut cursor);
            let opts = zip::write::SimpleFileOptions::default()
                .compression_method(zip::CompressionMethod::Deflated);
            w.start_file(&sha, opts).unwrap();
            w.write_all(payload).unwrap();
            w.finish().unwrap();
        }
        let bytes = cursor.into_inner();
        let dir = std::env::temp_dir().join(format!("kirivers-zip-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        ZipUnpacker
            .unpack(
                &bytes,
                &[IntegrityFile {
                    path: "nested/hi.txt".into(),
                    size: payload.len() as i64,
                    sha256: Some(sha),
                    md5: None,
                    url: None,
                    install_policy: Some("OVERWRITE".into()),
                    integrity_check: Some(true),
                }],
                &dir,
            )
            .unwrap();
        assert_eq!(
            std::fs::read(dir.join("nested").join("hi.txt")).unwrap(),
            payload
        );
        let _ = std::fs::remove_dir_all(&dir);
    }
}
