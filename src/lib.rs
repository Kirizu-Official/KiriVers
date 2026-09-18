//! Official KiriVers native JSON client SDK.
//!
//! Blocking HTTP (reqwest + rustls). Callers do not need a tokio runtime.
//! Protocol JSON uses serde; there is no injectable JSON codec.

mod adapters;
mod client;
mod config;
mod error;
mod filestore;
mod hasher;
mod patcher;
mod pathutil;
mod replacer;
mod signature;
mod transport;
mod types;
mod unpacker;
mod updater;

pub use adapters::{check_capabilities, Adapters};
pub use client::Client;
pub use config::{Config, VerifyKey};
pub use error::Error;
pub use filestore::{FileStore, StdFileStore};
pub use hasher::{Hasher, StdHasher};
pub use patcher::{detect_delta_magic, DeltaMagic, Patcher};
pub use pathutil::normalize_path;
pub use replacer::{RenameReplacer, Replacer};
pub use signature::{build_check_payload, SignatureVerifier, StdSignatureVerifier};
pub use transport::{
    FnTransport, HttpRequest, HttpResponse, RecordingTransport, ReqwestTransport, Transport,
};
pub use types::*;
pub use unpacker::{ArchiveUnpacker, ZipUnpacker};
pub use updater::{PackPollOptions, UpdateRequest, UpdateResult, UpdateStatus, Updater};

/// Snapshot identity: OpenAPI `info.version` plus a short digest of `openapi.client.json`.
pub const OPENAPI_REVISION: &str = include_str!("../OPENAPI_REVISION");

/// crates.io / User-Agent identity.
pub const SDK_VERSION: &str = env!("CARGO_PKG_VERSION");
