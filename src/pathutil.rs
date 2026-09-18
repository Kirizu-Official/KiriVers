use unicode_normalization::UnicodeNormalization;

use crate::error::Error;

/// Normalize a fileset path the same way as server `pkg/pathutil.NormalizeAndValidatePath`:
/// reject NUL/controls, drive letters, leading slashes, `.` / `..`; `\` → `/`; Unicode NFC.
pub fn normalize_path(raw: &str) -> Result<String, Error> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return Err(Error::Path("path is empty".into()));
    }
    for b in trimmed.bytes() {
        if b < 0x20 {
            return Err(Error::Path(format!("contains control character 0x{b:02x}")));
        }
    }
    if trimmed.starts_with('/') || trimmed.starts_with('\\') {
        return Err(Error::Path("path cannot start with leading slash".into()));
    }
    if has_drive_letter(trimmed) {
        return Err(Error::Path("path cannot contain drive letter".into()));
    }
    let slash_unified = trimmed.replace('\\', "/");
    for seg in slash_unified.split('/') {
        if has_drive_letter(seg) {
            return Err(Error::Path("segment cannot contain drive letter".into()));
        }
        if seg == "." || seg == ".." {
            return Err(Error::Path(format!(
                "path traversal segment {seg:?} is forbidden"
            )));
        }
    }
    let nfc: String = slash_unified.nfc().collect();
    let mut cleaned = String::with_capacity(nfc.len());
    let mut prev_slash = false;
    for ch in nfc.chars() {
        if ch == '/' {
            if prev_slash {
                continue;
            }
            prev_slash = true;
            cleaned.push('/');
        } else {
            prev_slash = false;
            cleaned.push(ch);
        }
    }
    let cleaned = cleaned.trim_matches('/').to_string();
    if cleaned.is_empty() {
        return Err(Error::Path("path normalized to empty".into()));
    }
    for seg in cleaned.split('/') {
        if seg == "." || seg == ".." || seg.is_empty() {
            return Err(Error::Path(format!(
                "invalid segment in normalized path {cleaned:?}"
            )));
        }
    }
    Ok(cleaned)
}

fn has_drive_letter(s: &str) -> bool {
    let b = s.as_bytes();
    b.len() >= 2 && b[0].is_ascii_alphabetic() && b[1] == b':'
}

/// Unique NFC paths; invalid entries are dropped (server reports `invalid_paths`).
#[cfg(test)]
pub fn unique_needed_paths<I, S>(paths: I) -> Vec<String>
where
    I: IntoIterator<Item = S>,
    S: AsRef<str>,
{
    let mut out = Vec::new();
    for p in paths {
        if let Ok(n) = normalize_path(p.as_ref()) {
            if !out.iter().any(|e| e == &n) {
                out.push(n);
            }
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn backslash_and_nfc_unify() {
        let a = normalize_path("foo\\bar").unwrap();
        let b = normalize_path("foo/bar").unwrap();
        assert_eq!(a, b);
        let nfd = "cafe\u{0301}/x";
        let nfc = "café/x";
        assert_eq!(normalize_path(nfd).unwrap(), normalize_path(nfc).unwrap());
    }

    #[test]
    fn rejects_dotdot() {
        assert!(normalize_path("a/../b").is_err());
        assert!(normalize_path("../b").is_err());
        assert!(normalize_path("/abs").is_err());
        assert!(normalize_path("C:\\windows").is_err());
    }

    #[test]
    fn unique_needed_paths_order_independent_identity() {
        let a = unique_needed_paths(["foo\\bar", "foo/bar", "baz"]);
        assert_eq!(a, vec!["foo/bar", "baz"]);
    }
}
