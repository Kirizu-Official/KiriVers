use std::path::Path;

use crate::error::Error;

/// Replace a running install after bytes are verified. Android/HarmonyOS APK install is caller-owned.
pub trait Replacer: Send + Sync {
    fn replace(&self, staged: &Path, dest: &Path) -> Result<(), Error>;
}

/// Default Replacer: `std::fs::rename`. Occupied / locked destinations fail with [`Error::ReplaceBusy`].
/// This is not a one-click installer for running binaries, APK sideload, or reboot replace.
#[derive(Debug, Default, Clone, Copy)]
pub struct RenameReplacer;

impl Replacer for RenameReplacer {
    fn replace(&self, staged: &Path, dest: &Path) -> Result<(), Error> {
        if let Some(parent) = dest.parent() {
            std::fs::create_dir_all(parent)?;
        }
        match std::fs::rename(staged, dest) {
            Ok(()) => Ok(()),
            Err(e) if is_busy(&e) => Err(Error::ReplaceBusy {
                dest: dest.to_path_buf(),
                source: e.to_string(),
            }),
            Err(e) => Err(e.into()),
        }
    }
}

fn is_busy(err: &std::io::Error) -> bool {
    #[cfg(windows)]
    {
        matches!(err.raw_os_error(), Some(5) | Some(32) | Some(33))
    }
    #[cfg(not(windows))]
    {
        matches!(err.kind(), std::io::ErrorKind::PermissionDenied)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rename_unlocked_file() {
        let dir = std::env::temp_dir().join(format!("kirivers-ren-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let staged = dir.join("staged.bin");
        let dest = dir.join("dest.bin");
        std::fs::write(&staged, b"abc").unwrap();
        RenameReplacer.replace(&staged, &dest).unwrap();
        assert_eq!(std::fs::read(&dest).unwrap(), b"abc");
        let _ = std::fs::remove_dir_all(&dir);
    }
}
