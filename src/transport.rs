use std::sync::{Arc, Mutex};

use crate::error::Error;

/// HTTP request the [`Transport`] must send. Query strings (including `exp`/`sig`) live on `url`.
#[derive(Clone, Debug)]
pub struct HttpRequest {
    pub method: String,
    pub url: String,
    pub headers: Vec<(String, String)>,
    pub body: Option<Vec<u8>>,
}

/// HTTP response bytes plus headers. Status 204 / 304 may have an empty body.
#[derive(Clone, Debug)]
pub struct HttpResponse {
    pub status: u16,
    pub headers: Vec<(String, String)>,
    pub body: Vec<u8>,
}

impl HttpResponse {
    pub fn header(&self, name: &str) -> Option<&str> {
        self.headers
            .iter()
            .find(|(k, _)| k.eq_ignore_ascii_case(name))
            .map(|(_, v)| v.as_str())
    }

    pub fn json<T: serde::de::DeserializeOwned>(&self) -> Result<T, Error> {
        serde_json::from_slice(&self.body).map_err(Error::from)
    }
}

/// Injectable HTTP stack. The default implementation is reqwest blocking + rustls.
pub trait Transport: Send + Sync {
    fn execute(&self, request: &HttpRequest) -> Result<HttpResponse, Error>;
}

/// Default Transport: reqwest `blocking` + `rustls-tls`. No hyper/ureq client is constructed.
pub struct ReqwestTransport {
    inner: reqwest::blocking::Client,
}

impl ReqwestTransport {
    pub fn new(timeout: std::time::Duration) -> Result<Self, Error> {
        let inner = reqwest::blocking::Client::builder()
            .use_rustls_tls()
            .timeout(timeout)
            .connect_timeout(timeout)
            .user_agent(format!("kirivers-client/{}", crate::SDK_VERSION))
            .redirect(reqwest::redirect::Policy::limited(10))
            .build()
            .map_err(|e| Error::Transport(e.to_string()))?;
        Ok(Self { inner })
    }
}

impl Transport for ReqwestTransport {
    fn execute(&self, request: &HttpRequest) -> Result<HttpResponse, Error> {
        let method = request.method.to_ascii_uppercase();
        let mut builder = match method.as_str() {
            "GET" => self.inner.get(&request.url),
            "HEAD" => self.inner.head(&request.url),
            "POST" => self.inner.post(&request.url),
            other => return Err(Error::UnsupportedMethod(other.to_string())),
        };
        for (k, v) in &request.headers {
            builder = builder.header(k.as_str(), v.as_str());
        }
        if let Some(body) = &request.body {
            builder = builder.body(body.clone());
        }
        let response = builder
            .send()
            .map_err(|e| Error::Transport(e.to_string()))?;
        let status = response.status().as_u16();
        let headers = response
            .headers()
            .iter()
            .map(|(k, v)| (k.to_string(), v.to_str().unwrap_or_default().to_string()))
            .collect();
        let body = response
            .bytes()
            .map_err(|e| Error::Transport(e.to_string()))?;
        Ok(HttpResponse {
            status,
            headers,
            body: body.to_vec(),
        })
    }
}

/// Closure-based Transport for contract tests (no listening server).
pub struct FnTransport<F> {
    f: F,
}

impl<F> FnTransport<F>
where
    F: Fn(&HttpRequest) -> Result<HttpResponse, Error> + Send + Sync,
{
    pub fn new(f: F) -> Self {
        Self { f }
    }
}

impl<F> Transport for FnTransport<F>
where
    F: Fn(&HttpRequest) -> Result<HttpResponse, Error> + Send + Sync,
{
    fn execute(&self, request: &HttpRequest) -> Result<HttpResponse, Error> {
        (self.f)(request)
    }
}

/// Records every request then delegates to `inner`.
pub struct RecordingTransport {
    pub inner: Arc<dyn Transport>,
    pub calls: Arc<Mutex<Vec<HttpRequest>>>,
}

impl RecordingTransport {
    pub fn new(inner: Arc<dyn Transport>) -> Self {
        Self {
            inner,
            calls: Arc::new(Mutex::new(Vec::new())),
        }
    }

    pub fn calls(&self) -> Vec<HttpRequest> {
        self.calls.lock().expect("recording mutex").clone()
    }
}

impl Transport for RecordingTransport {
    fn execute(&self, request: &HttpRequest) -> Result<HttpResponse, Error> {
        self.calls
            .lock()
            .expect("recording mutex")
            .push(request.clone());
        self.inner.execute(request)
    }
}
