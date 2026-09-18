use crate::error::Error;

/// Official delta container magics (`docs/delta-engines.md`). Never cross-decode.
pub const MAGIC_KVDIFFHP1: &[u8] = b"KVDIFFHP1\n";
pub const MAGIC_HDIFF13: &[u8] = b"HDIFF13&";
pub const MAGIC_BSDIFF40: &[u8] = b"BSDIFF40";
pub const MAGIC_VCDIFF: &[u8] = &[0xD6, 0xC3, 0xC4];

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DeltaMagic {
    /// Pure-Go KiriVers hdiffpatch container.
    KvDiffHp1,
    /// Official HDiffPatch `HDIFF13&`.
    Hdiff13,
    /// bsdiff4.
    Bsdiff40,
    /// RFC 3284 / xdelta3.
    Vcdiff,
}

impl DeltaMagic {
    pub fn algo(self) -> &'static str {
        match self {
            Self::KvDiffHp1 | Self::Hdiff13 => "hdiffpatch",
            Self::Bsdiff40 => "bsdiff",
            Self::Vcdiff => "xdelta3",
        }
    }
}

pub fn detect_delta_magic(delta: &[u8]) -> Result<DeltaMagic, Error> {
    if delta.starts_with(MAGIC_KVDIFFHP1) {
        Ok(DeltaMagic::KvDiffHp1)
    } else if delta.starts_with(MAGIC_HDIFF13) {
        Ok(DeltaMagic::Hdiff13)
    } else if delta.starts_with(MAGIC_BSDIFF40) {
        Ok(DeltaMagic::Bsdiff40)
    } else if delta.starts_with(MAGIC_VCDIFF) {
        Ok(DeltaMagic::Vcdiff)
    } else {
        Err(Error::UnknownDeltaMagic)
    }
}

/// Reject unknown magic and refuse to apply a container under the wrong `delta_algo`.
pub fn assert_magic_matches(delta_algo: &str, delta: &[u8]) -> Result<DeltaMagic, Error> {
    let magic = detect_delta_magic(delta)?;
    if magic.algo() != delta_algo {
        return Err(Error::UnknownDeltaMagic);
    }
    Ok(magic)
}

/// Apply one binary delta. Official SDK has no default implementation (interface only).
pub trait Patcher: Send + Sync {
    fn supported_algos(&self) -> Vec<String>;
    fn apply(&self, algo: &str, basis: &[u8], delta: &[u8]) -> Result<Vec<u8>, Error>;
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn magics() {
        assert_eq!(
            detect_delta_magic(b"KVDIFFHP1\nxxxx").unwrap().algo(),
            "hdiffpatch"
        );
        assert_eq!(
            detect_delta_magic(b"HDIFF13&xxxx").unwrap().algo(),
            "hdiffpatch"
        );
        assert_eq!(
            detect_delta_magic(b"BSDIFF40xxxx").unwrap().algo(),
            "bsdiff"
        );
        assert_eq!(
            detect_delta_magic(&[0xD6, 0xC3, 0xC4, 0x00])
                .unwrap()
                .algo(),
            "xdelta3"
        );
        assert!(detect_delta_magic(b"nope").is_err());
        assert!(assert_magic_matches("bsdiff", b"HDIFF13&x").is_err());
    }
}
