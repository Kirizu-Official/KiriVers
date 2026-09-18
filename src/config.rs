use std::time::Duration;

/// Caller-supplied client-plane configuration. The SDK never invents a `device_id`.
#[derive(Clone)]
pub struct Config {
    /// Client plane origin, no trailing slash (example: `http://127.0.0.1:8080`).
    pub base_url: String,
    /// Project UUID, live slug, or unexpired slug alias.
    pub project_ref: String,
    /// Optional project client token (`Authorization: Bearer` and `X-Project-Token`).
    pub project_token: Option<String>,
    /// Optional channel token (`X-Channel-Token`). Never logged.
    pub channel_token: Option<String>,
    /// HTTP timeout for the default reqwest transport.
    pub timeout: Duration,
    /// Public keys used when a response includes `signature`.
    pub verify_keys: Vec<VerifyKey>,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            base_url: String::new(),
            project_ref: String::new(),
            project_token: None,
            channel_token: None,
            timeout: Duration::from_secs(60),
            verify_keys: Vec::new(),
        }
    }
}

impl Config {
    pub fn new(base_url: impl Into<String>, project_ref: impl Into<String>) -> Self {
        Self {
            base_url: trim_base(base_url.into()),
            project_ref: project_ref.into(),
            ..Self::default()
        }
    }

    pub(crate) fn validate(&self) -> Result<(), crate::Error> {
        if self.base_url.is_empty() {
            return Err(crate::Error::Config("base_url is required".into()));
        }
        if self.project_ref.is_empty() {
            return Err(crate::Error::Config("project_ref is required".into()));
        }
        Ok(())
    }
}

impl std::fmt::Debug for Config {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Config")
            .field("base_url", &self.base_url)
            .field("project_ref", &self.project_ref)
            .field("has_project_token", &self.project_token.is_some())
            .field("has_channel_token", &self.channel_token.is_some())
            .field("timeout", &self.timeout)
            .field("verify_keys", &self.verify_keys.len())
            .finish()
    }
}

/// Project signing public key (PEM). Algorithms: `ed25519` or `rsa-sha256`.
#[derive(Clone, Debug)]
pub struct VerifyKey {
    pub algo: String,
    pub public_key_pem: String,
}

pub(crate) fn trim_base(url: String) -> String {
    url.trim().trim_end_matches('/').to_string()
}
