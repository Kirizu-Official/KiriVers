use std::sync::Arc;

use reqwest::Url;
use serde::de::DeserializeOwned;

use crate::config::Config;
use crate::error::Error;
use crate::transport::{HttpRequest, HttpResponse, ReqwestTransport, Transport};
use crate::types::*;

const UA: &str = concat!("kirivers-client/", env!("CARGO_PKG_VERSION"));

/// Typed native JSON client. URLs are built from `Config`; leftover/store paths are not exposed.
#[derive(Clone)]
pub struct Client {
    config: Config,
    transport: Arc<dyn Transport>,
}

impl Client {
    /// Default reqwest blocking + rustls Transport.
    pub fn new(config: Config) -> Result<Self, Error> {
        let mut config = config;
        config.base_url = crate::config::trim_base(config.base_url);
        config.validate()?;
        let transport = Arc::new(ReqwestTransport::new(config.timeout)?);
        Ok(Self { config, transport })
    }

    /// Inject a Transport (contract tests, custom stacks). Still requires `base_url` / `project_ref`.
    pub fn with_transport(config: Config, transport: Arc<dyn Transport>) -> Result<Self, Error> {
        let mut config = config;
        config.base_url = crate::config::trim_base(config.base_url);
        config.validate()?;
        Ok(Self { config, transport })
    }

    pub fn config(&self) -> &Config {
        &self.config
    }

    pub fn transport(&self) -> Arc<dyn Transport> {
        Arc::clone(&self.transport)
    }

    // --- native JSON operations ---

    pub fn project(&self) -> Result<ProjectPublic, Error> {
        let url = self.project_url(&[])?;
        let resp = self.execute("GET", &url, None, &[])?;
        self.expect_json(&resp, &[200])
    }

    pub fn device_report(&self, input: &DeviceReportInput) -> Result<DeviceReportOutput, Error> {
        if input.device_id.is_empty() {
            return Err(Error::Config(
                "device_id is required for device_report".into(),
            ));
        }
        let url = self.project_url(&["clients", "report"])?;
        let body = serde_json::to_vec(input)?;
        let resp = self.execute("POST", &url, Some(body), &[])?;
        self.expect_json(&resp, &[200])
    }

    pub fn check(&self, req: &CheckRequest) -> Result<CheckOutcome, Error> {
        self.check_with(req, &RequestOptions::default())
    }

    pub fn check_with(
        &self,
        req: &CheckRequest,
        opts: &RequestOptions,
    ) -> Result<CheckOutcome, Error> {
        if req.current_version.is_empty() || req.os.is_empty() || req.arch.is_empty() {
            return Err(Error::Config(
                "check requires current_version, os, and arch".into(),
            ));
        }
        let mut body_req = req.clone();
        if body_req.capabilities.is_empty() {
            body_req.capabilities = vec!["full_package".into()];
        }
        let url = self.project_url(&["update", "check"])?;
        let extra = etag_headers(opts);
        let resp = self.execute("POST", &url, Some(serde_json::to_vec(&body_req)?), &extra)?;
        let etag = resp.header("etag").map(ToOwned::to_owned);
        match resp.status {
            200 => Ok(CheckOutcome::Update {
                body: resp.json()?,
                etag,
            }),
            204 => Ok(CheckOutcome::NoUpdate { etag }),
            304 => Ok(CheckOutcome::NotModified { etag }),
            _ => Err(self.api_error(&resp)),
        }
    }

    pub fn changelog(&self, query: &ChangelogQuery) -> Result<Cached<ChangelogBody>, Error> {
        let mut pairs = Vec::new();
        push_q(&mut pairs, "from_version", query.from_version.as_deref());
        push_q(&mut pairs, "to_version", query.to_version.as_deref());
        push_q(
            &mut pairs,
            "changelog_scope",
            query.changelog_scope.as_deref(),
        );
        push_q(
            &mut pairs,
            "changelog_layout",
            query.changelog_layout.as_deref(),
        );
        if let Some(v) = query.changelog_include_revoked {
            pairs.push(("changelog_include_revoked".into(), bool_str(v)));
        }
        if let Some(v) = query.changelog_include_platform_notes {
            pairs.push(("changelog_include_platform_notes".into(), bool_str(v)));
        }
        push_q(
            &mut pairs,
            "changelog_locale",
            query.changelog_locale.as_deref(),
        );
        push_q(&mut pairs, "locale", query.locale.as_deref());
        let url = self.project_url_query(
            &["changelog", &query.channel, &query.os, &query.arch],
            &pairs,
        )?;
        let opts = RequestOptions {
            if_none_match: query.if_none_match.clone(),
            ..RequestOptions::default()
        };
        let extra = etag_headers(&opts);
        let resp = self.execute("GET", &url, None, &extra)?;
        self.cached_json(resp)
    }

    pub fn integrity(&self, query: &IntegrityQuery) -> Result<Cached<IntegrityBody>, Error> {
        let mut pairs = vec![
            ("os".into(), query.os.clone()),
            ("arch".into(), query.arch.clone()),
        ];
        push_q(&mut pairs, "hash_algo", query.hash_algo.as_deref());
        if let Some(v) = query.compact {
            pairs.push(("compact".into(), bool_str(v)));
        }
        if let Some(v) = query.include_file_urls {
            pairs.push(("include_file_urls".into(), bool_str(v)));
        }
        push_q(&mut pairs, "hw_rev", query.hw_rev.as_deref());
        push_q(&mut pairs, "channel", query.channel.as_deref());
        let url = self.project_url_query(&["versions", &query.version, "integrity"], &pairs)?;
        let opts = RequestOptions {
            if_none_match: query.if_none_match.clone(),
            ..RequestOptions::default()
        };
        let extra = etag_headers(&opts);
        let resp = self.execute("GET", &url, None, &extra)?;
        self.cached_json(resp)
    }

    pub fn diff(&self, req: &DiffRequest) -> Result<DiffResponse, Error> {
        let url = self.project_url(&["update", "diff"])?;
        let resp = self.execute("POST", &url, Some(serde_json::to_vec(req)?), &[])?;
        self.expect_json(&resp, &[200])
    }

    /// Single POST. For pending jobs use [`Self::pack_until_ready`] with the identical JSON.
    pub fn pack(&self, req: &PackRequest) -> Result<(u16, PackResponse), Error> {
        let body = serde_json::to_vec(req)?;
        self.pack_raw(&body)
    }

    pub fn pack_raw(&self, body: &[u8]) -> Result<(u16, PackResponse), Error> {
        let url = self.project_url(&["update", "pack"])?;
        let resp = self.execute("POST", &url, Some(body.to_vec()), &[])?;
        if resp.status != 200 && resp.status != 202 {
            return Err(self.api_error(&resp));
        }
        Ok((resp.status, resp.json()?))
    }

    /// POST once; if HTTP 202 or `status=pending`, wait and POST the **same bytes** until
    /// `ready` / `full_package` or the deadline.
    pub fn pack_until_ready(
        &self,
        req: &PackRequest,
        opts: crate::updater::PackPollOptions,
    ) -> Result<PackResponse, Error> {
        let body = serde_json::to_vec(req)?;
        self.pack_until_ready_raw(&body, opts)
    }

    pub fn pack_until_ready_raw(
        &self,
        body: &[u8],
        opts: crate::updater::PackPollOptions,
    ) -> Result<PackResponse, Error> {
        let start = std::time::Instant::now();
        let mut delay = opts.initial_delay;
        loop {
            let (status, pack) = self.pack_raw(body)?;
            let pending = status == 202 || pack.status == "pending";
            if !pending {
                return Ok(pack);
            }
            if start.elapsed() >= opts.deadline {
                return Err(Error::api(
                    408,
                    "PACK_TIMEOUT",
                    "pack poll deadline exceeded",
                ));
            }
            std::thread::sleep(delay);
            if delay < opts.max_delay {
                delay = (delay * 2).min(opts.max_delay);
            }
        }
    }

    /// GET a package URL (relative or absolute). Keeps `exp`/`sig`. Optional `Range`.
    pub fn download(&self, package_url: &str, range: Option<&str>) -> Result<Download, Error> {
        let url = self.resolve_url(package_url)?;
        let extra = range
            .map(|r| vec![("Range".to_string(), r.to_string())])
            .unwrap_or_default();
        let extra_ref: Vec<(&str, &str)> = extra
            .iter()
            .map(|(k, v)| (k.as_str(), v.as_str()))
            .collect();
        let resp = self.execute_raw("GET", &url, None, &extra_ref, false)?;
        if resp.status != 200 && resp.status != 206 {
            return Err(self.api_error(&resp));
        }
        Ok(Download {
            status: resp.status,
            headers: resp.headers,
            body: resp.body,
        })
    }

    pub fn download_package(
        &self,
        content_ref: &str,
        extra_query: &[(&str, &str)],
        range: Option<&str>,
    ) -> Result<Download, Error> {
        let pairs: Vec<(String, String)> = extra_query
            .iter()
            .map(|(k, v)| ((*k).to_string(), (*v).to_string()))
            .collect();
        let url = self.project_url_query(&["packages", content_ref], &pairs)?;
        self.download(&url, range)
    }

    pub fn head_package(
        &self,
        content_ref: &str,
        extra_query: &[(&str, &str)],
    ) -> Result<HttpResponse, Error> {
        let pairs: Vec<(String, String)> = extra_query
            .iter()
            .map(|(k, v)| ((*k).to_string(), (*v).to_string()))
            .collect();
        let url = self.project_url_query(&["packages", content_ref], &pairs)?;
        let resp = self.execute_raw("HEAD", &url, None, &[], false)?;
        if resp.status != 200 && resp.status != 206 {
            return Err(self.api_error(&resp));
        }
        Ok(resp)
    }

    pub fn head_url(&self, package_url: &str) -> Result<HttpResponse, Error> {
        let url = self.resolve_url(package_url)?;
        let resp = self.execute_raw("HEAD", &url, None, &[], false)?;
        if resp.status != 200 && resp.status != 206 {
            return Err(self.api_error(&resp));
        }
        Ok(resp)
    }

    pub fn channels(&self) -> Result<Vec<Channel>, Error> {
        let url = self.project_url(&["channels"])?;
        let resp = self.execute("GET", &url, None, &[])?;
        let list: ChannelList = self.expect_json(&resp, &[200])?;
        Ok(list.channels)
    }

    pub fn matrix(&self) -> Result<Vec<MatrixRow>, Error> {
        let url = self.project_url(&["matrix"])?;
        let resp = self.execute("GET", &url, None, &[])?;
        let list: MatrixList = self.expect_json(&resp, &[200])?;
        Ok(list.matrix)
    }

    pub fn languages(&self) -> Result<Vec<Language>, Error> {
        let url = self.project_url(&["languages"])?;
        let resp = self.execute("GET", &url, None, &[])?;
        let list: LanguageList = self.expect_json(&resp, &[200])?;
        Ok(list.languages)
    }

    pub fn announcements(
        &self,
        query: &AnnouncementQuery,
    ) -> Result<Cached<Vec<Announcement>>, Error> {
        let mut pairs = Vec::new();
        push_q(&mut pairs, "version", query.version.as_deref());
        push_q(&mut pairs, "os", query.os.as_deref());
        push_q(&mut pairs, "arch", query.arch.as_deref());
        push_q(&mut pairs, "locale", query.locale.as_deref());
        let url = self.project_url_query(&["announcements"], &pairs)?;
        let opts = RequestOptions {
            if_none_match: query.if_none_match.clone(),
            accept_language: query.accept_language.clone(),
            ..RequestOptions::default()
        };
        let extra = etag_headers(&opts);
        let resp = self.execute("GET", &url, None, &extra)?;
        match resp.status {
            200 => {
                let list: AnnouncementList = resp.json()?;
                Ok(Cached::Fresh {
                    value: list.announcements,
                    etag: resp.header("etag").map(ToOwned::to_owned),
                })
            }
            304 => Ok(Cached::NotModified {
                etag: resp.header("etag").map(ToOwned::to_owned),
            }),
            _ => Err(self.api_error(&resp)),
        }
    }

    /// Telemetry is 202. Callers of Update must ignore failures here.
    pub fn report_telemetry(&self, report: &TelemetryReport) -> Result<(), Error> {
        let url = self.project_url(&["telemetry", "report"])?;
        let resp = self.execute("POST", &url, Some(serde_json::to_vec(report)?), &[])?;
        if resp.status == 202 {
            Ok(())
        } else {
            Err(self.api_error(&resp))
        }
    }

    pub fn media(&self, id: &str, range: Option<&str>) -> Result<Download, Error> {
        let url = self.project_url(&["media", id])?;
        self.download(&url, range)
    }

    pub fn head_media(&self, id: &str) -> Result<HttpResponse, Error> {
        let url = self.project_url(&["media", id])?;
        let resp = self.execute_raw("HEAD", &url, None, &[], false)?;
        if resp.status != 200 {
            return Err(self.api_error(&resp));
        }
        Ok(resp)
    }

    pub fn health(&self) -> Result<Health, Error> {
        let url = self.root_url(&["api", "v1", "health"])?;
        let resp = self.execute("GET", &url, None, &[])?;
        self.expect_json(&resp, &[200])
    }

    // --- internals ---

    fn execute(
        &self,
        method: &str,
        url: &str,
        body: Option<Vec<u8>>,
        extra: &[(&str, &str)],
    ) -> Result<HttpResponse, Error> {
        self.execute_raw(method, url, body, extra, true)
    }

    fn execute_raw(
        &self,
        method: &str,
        url: &str,
        body: Option<Vec<u8>>,
        extra: &[(&str, &str)],
        json_accept: bool,
    ) -> Result<HttpResponse, Error> {
        let mut headers = Vec::new();
        headers.push(("User-Agent".into(), UA.to_string()));
        if json_accept {
            headers.push(("Accept".into(), "application/json".into()));
        }
        if body.is_some() {
            headers.push(("Content-Type".into(), "application/json".into()));
        }
        if let Some(t) = &self.config.project_token {
            headers.push(("Authorization".into(), format!("Bearer {t}")));
            headers.push(("X-Project-Token".into(), t.clone()));
        }
        if let Some(t) = &self.config.channel_token {
            headers.push(("X-Channel-Token".into(), t.clone()));
        }
        for (k, v) in extra {
            headers.push(((*k).to_string(), (*v).to_string()));
        }
        self.transport.execute(&HttpRequest {
            method: method.to_string(),
            url: url.to_string(),
            headers,
            body,
        })
    }

    fn expect_json<T: DeserializeOwned>(
        &self,
        resp: &HttpResponse,
        ok: &[u16],
    ) -> Result<T, Error> {
        if !ok.contains(&resp.status) {
            return Err(self.api_error(resp));
        }
        resp.json()
    }

    fn cached_json<T: DeserializeOwned>(&self, resp: HttpResponse) -> Result<Cached<T>, Error> {
        let etag = resp.header("etag").map(ToOwned::to_owned);
        match resp.status {
            200 => Ok(Cached::Fresh {
                value: resp.json()?,
                etag,
            }),
            304 => Ok(Cached::NotModified { etag }),
            _ => Err(self.api_error(&resp)),
        }
    }

    pub(crate) fn api_error(&self, resp: &HttpResponse) -> Error {
        let retry_after = resp.header("retry-after").map(ToOwned::to_owned);
        if let Ok(env) = serde_json::from_slice::<ErrorEnvelope>(&resp.body) {
            if let Some(e) = env.error {
                return Error::Api {
                    status: resp.status,
                    code: if e.code.is_empty() {
                        "UNKNOWN".into()
                    } else {
                        e.code
                    },
                    message: e.message,
                    details: e.details,
                    retry_after,
                };
            }
        }
        let snippet = String::from_utf8_lossy(&resp.body);
        Error::Api {
            status: resp.status,
            code: "UNKNOWN".into(),
            message: snippet.chars().take(512).collect(),
            details: None,
            retry_after,
        }
    }

    fn project_url(&self, rest: &[&str]) -> Result<String, Error> {
        self.project_url_query(rest, &[])
    }

    fn project_url_query(
        &self,
        rest: &[&str],
        query: &[(String, String)],
    ) -> Result<String, Error> {
        let mut parts = vec!["api", "v1", "projects", self.config.project_ref.as_str()];
        parts.extend(rest.iter().copied());
        let mut url = self.root_url(&parts)?;
        if !query.is_empty() {
            let mut parsed = Url::parse(&url).map_err(|e| Error::Config(e.to_string()))?;
            {
                let mut q = parsed.query_pairs_mut();
                for (k, v) in query {
                    q.append_pair(k, v);
                }
            }
            url = parsed.to_string();
        }
        Ok(url)
    }

    fn root_url(&self, parts: &[&str]) -> Result<String, Error> {
        let mut url = Url::parse(&self.config.base_url)
            .map_err(|e| Error::Config(format!("base_url: {e}")))?;
        {
            let mut segs = url
                .path_segments_mut()
                .map_err(|_| Error::Config("base_url cannot be a cannot-be-a-base URL".into()))?;
            // Keep any existing prefix, then append.
            for p in parts {
                if !p.is_empty() {
                    segs.push(p);
                }
            }
        }
        Ok(url.to_string())
    }

    pub(crate) fn resolve_url(&self, maybe_relative: &str) -> Result<String, Error> {
        if maybe_relative.starts_with("http://") || maybe_relative.starts_with("https://") {
            return Ok(maybe_relative.to_string());
        }
        let base = Url::parse(&self.config.base_url).map_err(|e| Error::Config(e.to_string()))?;
        Ok(base
            .join(maybe_relative)
            .map_err(|e| Error::Config(e.to_string()))?
            .to_string())
    }
}

fn etag_headers(opts: &RequestOptions) -> Vec<(&str, &str)> {
    let mut v = Vec::new();
    if let Some(t) = opts.if_none_match.as_deref() {
        v.push(("If-None-Match", t));
    }
    if let Some(r) = opts.range.as_deref() {
        v.push(("Range", r));
    }
    if let Some(a) = opts.accept_language.as_deref() {
        v.push(("Accept-Language", a));
    }
    v
}

fn push_q(pairs: &mut Vec<(String, String)>, key: &str, val: Option<&str>) {
    if let Some(v) = val {
        if !v.is_empty() {
            pairs.push((key.to_string(), v.to_string()));
        }
    }
}

fn bool_str(v: bool) -> String {
    if v {
        "true".into()
    } else {
        "false".into()
    }
}
