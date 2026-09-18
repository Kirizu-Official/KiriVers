use std::io::Read;
use std::path::Path;

use sha2::Digest;

use crate::error::Error;

/// SHA-256 (and MD5 when integrity `hash_algo` is `md5` or `both`).
pub trait Hasher: Send + Sync {
    fn sha256_hex(&self, data: &[u8]) -> String;
    fn sha256_reader(&self, reader: &mut dyn Read) -> Result<String, Error>;
    fn md5_hex(&self, data: &[u8]) -> String;

    fn sha256_file(&self, path: &Path) -> Result<String, Error> {
        let mut f = std::fs::File::open(path)?;
        self.sha256_reader(&mut f)
    }
}

/// Default Hasher: `sha2` + `md-5`.
#[derive(Debug, Default, Clone, Copy)]
pub struct StdHasher;

impl Hasher for StdHasher {
    fn sha256_hex(&self, data: &[u8]) -> String {
        let d = sha2::Sha256::digest(data);
        hex_lower(&d)
    }

    fn sha256_reader(&self, reader: &mut dyn Read) -> Result<String, Error> {
        let mut hasher = sha2::Sha256::new();
        let mut buf = [0u8; 8192];
        loop {
            let n = reader.read(&mut buf)?;
            if n == 0 {
                break;
            }
            hasher.update(&buf[..n]);
        }
        Ok(hex_lower(&hasher.finalize()))
    }

    fn md5_hex(&self, data: &[u8]) -> String {
        let d = md5::Md5::digest(data);
        hex_lower(&d)
    }
}

pub(crate) fn hex_lower(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for b in bytes {
        out.push(HEX[(b >> 4) as usize] as char);
        out.push(HEX[(b & 0x0f) as usize] as char);
    }
    out
}

pub(crate) fn eq_hex(expected: &str, actual: &str) -> bool {
    expected.eq_ignore_ascii_case(actual)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sha256_empty() {
        let h = StdHasher;
        assert_eq!(
            h.sha256_hex(b""),
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
        );
    }
}
