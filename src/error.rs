use std::fmt;
use std::io;
use std::path::PathBuf;

use serde_json::Value;

/// SDK failure. HTTP errors keep the `{ "error": { "code", "message", "details" } }` envelope.
/// Unknown `code` values stay opaque strings. This type never stores a caller `device_id`.
#[derive(Debug)]
pub enum Error {
    /// Parsed client-plane error envelope (any status, including 429 `RATE_LIMITED`).
    Api {
        status: u16,
        code: String,
        message: String,
        details: Option<Value>,
        retry_after: Option<String>,
    },
    /// Transport / DNS / TLS failure.
    Transport(String),
    /// JSON encode or decode.
    Json(String),
    /// Local filesystem.
    Io(io::Error),
    /// SHA-256 (or MD5) of downloaded bytes did not match the catalog.
    HashMismatch { expected: String, actual: String },
    /// `signature` did not verify over [`crate::build_check_payload`].
    Signature(String),
    /// Delta bytes had no known magic, or magic did not match `delta_algo`.
    UnknownDeltaMagic,
    /// Relative path failed NFC / traversal checks.
    Path(String),
    /// Missing required configuration.
    Config(String),
    /// `std::fs::rename` could not replace a locked destination; inject a `Replacer`.
    ReplaceBusy { dest: PathBuf, source: String },
    /// Caller asked for an HTTP method this transport does not implement.
    UnsupportedMethod(String),
}

impl Error {
    pub fn api(status: u16, code: impl Into<String>, message: impl Into<String>) -> Self {
        Self::Api {
            status,
            code: code.into(),
            message: message.into(),
            details: None,
            retry_after: None,
        }
    }

    /// Stable machine token when this is an envelope error.
    pub fn code(&self) -> Option<&str> {
        match self {
            Self::Api { code, .. } => Some(code.as_str()),
            _ => None,
        }
    }

    pub fn is_rate_limited(&self) -> bool {
        matches!(self, Self::Api { code, .. } if code == "RATE_LIMITED")
    }
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Api {
                status,
                code,
                message,
                retry_after,
                ..
            } => {
                write!(f, "HTTP {status} {code}: {message}")?;
                if let Some(ra) = retry_after {
                    write!(f, " (Retry-After: {ra})")?;
                }
                Ok(())
            }
            Self::Transport(m) => write!(f, "transport: {m}"),
            Self::Json(m) => write!(f, "json: {m}"),
            Self::Io(e) => write!(f, "io: {e}"),
            Self::HashMismatch { expected, actual } => {
                write!(f, "hash mismatch: expected {expected}, got {actual}")
            }
            Self::Signature(m) => write!(f, "signature: {m}"),
            Self::UnknownDeltaMagic => write!(f, "unknown or mismatched delta magic"),
            Self::Path(m) => write!(f, "path: {m}"),
            Self::Config(m) => write!(f, "config: {m}"),
            Self::ReplaceBusy { dest, source } => {
                write!(
                    f,
                    "replace busy ({source}): {} — inject a Replacer for locked files",
                    dest.display()
                )
            }
            Self::UnsupportedMethod(m) => write!(f, "unsupported HTTP method: {m}"),
        }
    }
}

impl std::error::Error for Error {}

impl From<io::Error> for Error {
    fn from(value: io::Error) -> Self {
        Self::Io(value)
    }
}

impl From<serde_json::Error> for Error {
    fn from(value: serde_json::Error) -> Self {
        Self::Json(value.to_string())
    }
}
