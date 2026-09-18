//! Contract tests: handwritten client vs `openapi.client.json`. Mock Transport only (no Docker).

use std::sync::{Arc, Mutex};
use std::time::Duration;

use kirivers_client::*;
use serde_json::{json, Value};

const OPENAPI: &str = include_str!("../openapi.client.json");

const NATIVE: &[(&str, &str)] = &[
    ("get", "/api/v1/health"),
    ("get", "/api/v1/projects/{project_ref}"),
    ("post", "/api/v1/projects/{project_ref}/clients/report"),
    ("post", "/api/v1/projects/{project_ref}/update/check"),
    (
        "get",
        "/api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}",
    ),
    (
        "get",
        "/api/v1/projects/{project_ref}/versions/{version}/integrity",
    ),
    ("post", "/api/v1/projects/{project_ref}/update/diff"),
    ("post", "/api/v1/projects/{project_ref}/update/pack"),
    ("get", "/api/v1/projects/{project_ref}/packages/{ref}"),
    ("head", "/api/v1/projects/{project_ref}/packages/{ref}"),
    ("post", "/api/v1/projects/{project_ref}/telemetry/report"),
    ("get", "/api/v1/projects/{project_ref}/channels"),
    ("get", "/api/v1/projects/{project_ref}/matrix"),
    ("get", "/api/v1/projects/{project_ref}/languages"),
    ("get", "/api/v1/projects/{project_ref}/announcements"),
    ("get", "/api/v1/projects/{project_ref}/media/{id}"),
    ("head", "/api/v1/projects/{project_ref}/media/{id}"),
];

fn spec_paths() -> Value {
    serde_json::from_str(OPENAPI).expect("openapi json")
}

#[test]
fn openapi_revision_mentions_100() {
    assert!(kirivers_client::OPENAPI_REVISION.contains("1.0.0"));
    assert_eq!(spec_paths()["info"]["version"].as_str(), Some("1.0.0"));
}

#[test]
fn openapi_contains_native_methods() {
    let spec = spec_paths();
    let paths = spec["paths"].as_object().unwrap();
    for (method, path) in NATIVE {
        let item = paths
            .get(*path)
            .unwrap_or_else(|| panic!("missing OpenAPI path {path}"));
        assert!(
            item.get(*method).is_some(),
            "OpenAPI {path} missing method {method}"
        );
    }
}

struct Script {
    calls: Mutex<Vec<HttpRequest>>,
    replies: Mutex<Vec<HttpResponse>>,
}

impl Transport for Script {
    fn execute(&self, request: &HttpRequest) -> Result<HttpResponse, Error> {
        self.calls.lock().unwrap().push(request.clone());
        let mut q = self.replies.lock().unwrap();
        if q.is_empty() {
            return Ok(json_resp(200, json!({})));
        }
        Ok(q.remove(0))
    }
}

fn json_resp(status: u16, body: Value) -> HttpResponse {
    HttpResponse {
        status,
        headers: vec![("content-type".into(), "application/json".into())],
        body: serde_json::to_vec(&body).unwrap(),
    }
}

fn bytes_resp(status: u16, body: Vec<u8>) -> HttpResponse {
    HttpResponse {
        status,
        headers: vec![],
        body,
    }
}

fn client_with(replies: Vec<HttpResponse>) -> (Client, Arc<Script>) {
    let script = Arc::new(Script {
        calls: Mutex::new(Vec::new()),
        replies: Mutex::new(replies),
    });
    let c = Client::with_transport(
        Config::new("http://example.test", "sdk-fixture"),
        script.clone() as Arc<dyn Transport>,
    )
    .unwrap();
    (c, script)
}

fn last_url(script: &Script) -> String {
    script.calls.lock().unwrap().last().unwrap().url.clone()
}

fn last(script: &Script) -> HttpRequest {
    script.calls.lock().unwrap().last().unwrap().clone()
}

fn assert_no_leftovers(calls: &[HttpRequest]) {
    for c in calls {
        let u = c.url.as_str();
        let m = c.method.to_ascii_uppercase();
        assert!(
            !(m == "GET" && u.contains("/update/check")),
            "leftover GET check: {m} {u}"
        );
        assert!(!u.contains("/clients/login"), "leftover login: {u}");
        assert!(!u.contains("/manifest"), "leftover manifest: {u}");
        assert!(!u.contains("/pack/status"), "leftover pack status: {u}");
        assert!(!u.contains("/store/"), "store feed: {u}");
        assert!(!u.contains("/artifacts/"), "leftover artifacts: {u}");
        assert!(
            !u.ends_with("/api/v1/ready") && !u.contains("/api/v1/ready?"),
            "leftover /ready: {u}"
        );
        if let Some(idx) = u.find("/channels/") {
            let rest = &u[idx + "/channels/".len()..];
            let slug = rest.split(['/', '?']).next().unwrap_or("");
            assert!(slug.is_empty(), "leftover GET /channels/{{slug}}: {u}");
        }
    }
}

#[test]
fn native_methods_emit_openapi_paths() {
    let check200 = json!({
        "has_update": true,
        "is_mandatory": false,
        "is_downgrade": false,
        "reason": "normal",
        "compare_engine": "semver",
        "version_integer": null,
        "version_semver": "1.1.0",
        "target_channel": "stable",
        "target_hw_rev": null,
        "package_type": "single_file",
        "root_hash": "",
        "package_url": "/api/v1/projects/sdk-fixture/packages/abc",
        "file_name": "a.bin",
        "size": 1,
        "sha256": "aa",
        "delta_available": false
    });
    let replies = vec![
        json_resp(200, json!({"slug": "sdk-fixture"})),
        json_resp(
            200,
            json!({"ip":"127.0.0.1","country_code":"","region_code":"","geo_i18n":{}}),
        ),
        json_resp(200, check200),
        json_resp(200, json!({"changelog": "hi"})),
        json_resp(
            200,
            json!({
                "version_integer": null,
                "version_semver": "1.1.0",
                "channel": "stable",
                "package_type": "single_file",
                "root_hash": "",
                "full_package_url": "/p",
                "file_name": "a.bin",
                "size": 1,
                "sha256": "aa",
                "files": []
            }),
        ),
        json_resp(
            200,
            json!({
                "diff_mode": "full_package",
                "root_hash": "",
                "version_integer": null,
                "version_semver": "1.1.0",
                "channel": "stable",
                "compare_engine": "semver"
            }),
        ),
        json_resp(202, json!({"status": "pending"})),
        bytes_resp(200, b"pkg".to_vec()),
        bytes_resp(200, vec![]),
        json_resp(202, json!({"status": "accepted"})),
        json_resp(200, json!({"channels": []})),
        json_resp(200, json!({"matrix": []})),
        json_resp(200, json!({"languages": []})),
        json_resp(200, json!({"announcements": []})),
        bytes_resp(200, b"img".to_vec()),
        bytes_resp(200, vec![]),
        json_resp(200, json!({"ready": true, "status": "ok"})),
    ];
    let (c, script) = client_with(replies);

    c.project().unwrap();
    assert!(last_url(&script).ends_with("/api/v1/projects/sdk-fixture"));
    assert_eq!(last(&script).method, "GET");

    c.device_report(&DeviceReportInput {
        device_id: "dev-1".into(),
        ..DeviceReportInput::default()
    })
    .unwrap();
    assert!(last_url(&script).contains("/clients/report"));
    assert_eq!(last(&script).method, "POST");
    let report: Value = serde_json::from_slice(last(&script).body.as_ref().unwrap()).unwrap();
    assert_eq!(report["device_id"], "dev-1");

    c.check(&CheckRequest {
        current_version: "1.0.0".into(),
        os: "windows".into(),
        arch: "x86_64".into(),
        ..CheckRequest::default()
    })
    .unwrap();
    assert!(last_url(&script).contains("/update/check"));
    assert_eq!(last(&script).method, "POST");
    let check_body: Value = serde_json::from_slice(last(&script).body.as_ref().unwrap()).unwrap();
    assert_eq!(check_body["current_version"], "1.0.0");
    assert_eq!(check_body["os"], "windows");
    assert_eq!(check_body["arch"], "x86_64");
    assert_eq!(check_body["capabilities"], json!(["full_package"]));
    assert!(check_body.get("local_sha256").is_none());
    assert!(check_body.get("dirty_paths").is_none());
    assert!(check_body.get("changelog_scope").is_none());

    c.changelog(&ChangelogQuery {
        channel: "stable".into(),
        os: "windows".into(),
        arch: "x86_64".into(),
        from_version: Some("1.0.0".into()),
        ..ChangelogQuery::default()
    })
    .unwrap();
    let u = last_url(&script);
    assert!(u.contains("/changelog/stable/windows/x86_64"));
    assert!(u.contains("from_version=1.0.0"));
    assert_eq!(last(&script).method, "GET");

    c.integrity(&IntegrityQuery {
        version: "1.1.0".into(),
        os: "windows".into(),
        arch: "x86_64".into(),
        ..IntegrityQuery::default()
    })
    .unwrap();
    let u = last_url(&script);
    assert!(u.contains("/versions/1.1.0/integrity"));
    assert!(u.contains("os=windows"));
    assert!(u.contains("arch=x86_64"));

    c.diff(&DiffRequest {
        source_version: "1.0.0".into(),
        target_version: "1.1.0".into(),
        os: "windows".into(),
        arch: "x86_64".into(),
        local_sha256: Some("abc".into()),
        ..DiffRequest::default()
    })
    .unwrap();
    assert!(last_url(&script).contains("/update/diff"));
    let diff_body: Value = serde_json::from_slice(last(&script).body.as_ref().unwrap()).unwrap();
    assert_eq!(diff_body["local_sha256"], "abc");
    assert_eq!(diff_body["source_version"], "1.0.0");

    c.pack(&PackRequest {
        source_version: "1.0.0".into(),
        target_version: "1.1.0".into(),
        os: "windows".into(),
        arch: "x86_64".into(),
        needed_paths: vec!["a/b".into()],
        ..PackRequest::default()
    })
    .unwrap();
    assert!(last_url(&script).contains("/update/pack"));
    assert!(!last_url(&script).contains("/pack/status"));

    c.download_package("deadbeef", &[("exp", "1"), ("sig", "s")], Some("bytes=0-1"))
        .unwrap();
    let req = last(&script);
    assert!(req.url.contains("/packages/deadbeef"));
    assert!(req.url.contains("exp=1"));
    assert!(req.url.contains("sig=s"));
    assert_eq!(req.method, "GET");
    assert!(req
        .headers
        .iter()
        .any(|(k, v)| k.eq_ignore_ascii_case("range") && v == "bytes=0-1"));

    c.head_package("deadbeef", &[]).unwrap();
    assert_eq!(last(&script).method, "HEAD");

    c.report_telemetry(&TelemetryReport {
        os: "windows".into(),
        arch: "x86_64".into(),
        channel: "stable".into(),
        from_version: "1.0.0".into(),
        to_version: "1.1.0".into(),
        status: "installed".into(),
        device_id: None,
        diff_mode: None,
        error_code: None,
        error_message: None,
    })
    .unwrap();
    assert!(last_url(&script).contains("/telemetry/report"));

    c.channels().unwrap();
    assert!(last_url(&script).ends_with("/channels"));
    c.matrix().unwrap();
    assert!(last_url(&script).ends_with("/matrix"));
    c.languages().unwrap();
    assert!(last_url(&script).ends_with("/languages"));
    c.announcements(&AnnouncementQuery::default()).unwrap();
    assert!(last_url(&script).contains("/announcements"));
    c.media("11111111-1111-1111-1111-111111111111", None)
        .unwrap();
    assert!(last_url(&script).contains("/media/"));
    c.head_media("11111111-1111-1111-1111-111111111111")
        .unwrap();
    assert_eq!(last(&script).method, "HEAD");
    c.health().unwrap();
    assert!(last_url(&script).ends_with("/api/v1/health"));

    let calls = script.calls.lock().unwrap().clone();
    assert_no_leftovers(&calls);
}

#[test]
fn error_envelope_preserves_code() {
    let (c, _) = client_with(vec![json_resp(
        404,
        json!({"error":{"code":"PROJECT_NOT_FOUND","message":"nope","details":null}}),
    )]);
    let err = c.project().unwrap_err();
    assert_eq!(err.code(), Some("PROJECT_NOT_FOUND"));
    match err {
        Error::Api { status, .. } => assert_eq!(status, 404),
        other => panic!("{other:?}"),
    }
}

#[test]
fn unknown_error_code_is_opaque() {
    let (c, _) = client_with(vec![json_resp(
        400,
        json!({"error":{"code":"NEW_SERVER_CODE","message":"x"}}),
    )]);
    let err = c.project().unwrap_err();
    assert_eq!(err.code(), Some("NEW_SERVER_CODE"));
}

#[test]
fn check_204_and_304_are_not_errors() {
    let (c, _) = client_with(vec![
        HttpResponse {
            status: 204,
            headers: vec![("etag".into(), "\"abc\"".into())],
            body: vec![],
        },
        HttpResponse {
            status: 304,
            headers: vec![("etag".into(), "\"abc\"".into())],
            body: vec![],
        },
    ]);
    let req = CheckRequest {
        current_version: "1.0.0".into(),
        os: "linux".into(),
        arch: "arm64".into(),
        ..CheckRequest::default()
    };
    match c.check(&req).unwrap() {
        CheckOutcome::NoUpdate { etag } => assert_eq!(etag.as_deref(), Some("\"abc\"")),
        other => panic!("{other:?}"),
    }
    match c
        .check_with(
            &req,
            &RequestOptions {
                if_none_match: Some("\"abc\"".into()),
                ..RequestOptions::default()
            },
        )
        .unwrap()
    {
        CheckOutcome::NotModified { .. } => {}
        other => panic!("{other:?}"),
    }
}

#[test]
fn pack_poll_repeats_identical_body() {
    let (c, script) = client_with(vec![
        json_resp(202, json!({"status": "pending"})),
        json_resp(
            200,
            json!({"status": "ready", "package_url": "/p", "sha256": "aa"}),
        ),
    ]);
    let req = PackRequest {
        source_version: "1.0.0".into(),
        target_version: "1.1.0".into(),
        os: "windows".into(),
        arch: "x86_64".into(),
        needed_paths: vec!["b".into(), "a".into()],
        ..PackRequest::default()
    };
    let pack = c
        .pack_until_ready(
            &req,
            PackPollOptions {
                initial_delay: Duration::from_millis(0),
                max_delay: Duration::from_millis(0),
                deadline: Duration::from_secs(5),
            },
        )
        .unwrap();
    assert_eq!(pack.status, "ready");
    let calls = script.calls.lock().unwrap();
    assert_eq!(calls.len(), 2);
    assert_eq!(calls[0].body, calls[1].body);
    assert!(calls[0].url.contains("/update/pack"));
    assert_eq!(calls[0].method, "POST");
}

#[test]
fn updater_without_patcher_does_not_send_binary_delta() {
    let hasher = StdHasher;
    let payload = b"full";
    let hex = hasher.sha256_hex(payload);
    let check200 = json!({
        "has_update": true,
        "is_mandatory": false,
        "is_downgrade": false,
        "reason": "normal",
        "compare_engine": "semver",
        "version_integer": null,
        "version_semver": "1.1.0",
        "target_channel": "stable",
        "target_hw_rev": null,
        "package_type": "single_file",
        "root_hash": "",
        "package_url": "/api/v1/projects/sdk-fixture/packages/aa",
        "file_name": "a.bin",
        "size": 4,
        "sha256": hex,
        "delta_available": false
    });

    let script = Arc::new(Script {
        calls: Mutex::new(Vec::new()),
        replies: Mutex::new(vec![
            json_resp(200, check200),
            bytes_resp(200, payload.to_vec()),
            json_resp(202, json!({"status": "ok"})),
        ]),
    });
    let client = Client::with_transport(
        Config::new("http://example.test", "sdk-fixture"),
        script.clone() as Arc<dyn Transport>,
    )
    .unwrap();
    let updater = Updater::new(client, Adapters::defaults());
    let dir = std::env::temp_dir().join(format!("kirivers-up-{}", std::process::id()));
    let _ = std::fs::create_dir_all(&dir);
    updater
        .run(&UpdateRequest {
            current_version: "1.0.0".into(),
            os: "windows".into(),
            arch: "x86_64".into(),
            channel: Some("stable".into()),
            device_id: Some("sdk-rust-test".into()),
            hw_rev: None,
            os_version: None,
            custom: None,
            report_device: false,
            if_none_match: None,
            install_dir: None,
            local_file: None,
            stage_dir: dir.clone(),
            dest: None,
            pack_poll: PackPollOptions::default(),
        })
        .unwrap();
    let calls = script.calls.lock().unwrap();
    let first = &calls[0];
    let body: Value = serde_json::from_slice(first.body.as_ref().unwrap()).unwrap();
    let caps = body["capabilities"].as_array().unwrap();
    assert!(caps.iter().any(|c| c == "full_package"));
    assert!(!caps.iter().any(|c| c == "binary_delta"));
    let empty_algos = body["accepted_delta_algos"].is_null()
        || body["accepted_delta_algos"]
            .as_array()
            .map(|a| a.is_empty())
            .unwrap_or(true);
    assert!(empty_algos);
    let tel_req = calls.last().expect("telemetry after update");
    assert!(tel_req.url.contains("/telemetry/report"));
    let tel: Value = serde_json::from_slice(tel_req.body.as_ref().unwrap()).unwrap();
    assert_eq!(tel["status"], "installed");
    assert_eq!(tel["diff_mode"], "full_package");
    drop(calls);
    let _ = std::fs::remove_dir_all(&dir);
}

struct FlagPatcher;

impl Patcher for FlagPatcher {
    fn supported_algos(&self) -> Vec<String> {
        vec!["bsdiff".into()]
    }
    fn apply(&self, _a: &str, _b: &[u8], _d: &[u8]) -> Result<Vec<u8>, Error> {
        unreachable!()
    }
}

#[test]
fn injected_patcher_advertises_binary_delta() {
    let mut adapters = Adapters::defaults();
    adapters.patcher = Some(Arc::new(FlagPatcher));
    let (caps, algos) = check_capabilities(&adapters);
    assert!(caps.contains(&"binary_delta".to_string()));
    assert_eq!(algos, vec!["bsdiff"]);
}

#[test]
fn auth_headers_and_bearer() {
    let (c, script) = {
        let script = Arc::new(Script {
            calls: Mutex::new(Vec::new()),
            replies: Mutex::new(vec![json_resp(200, json!({"slug": "x"}))]),
        });
        let mut cfg = Config::new("http://example.test", "sdk-fixture");
        cfg.project_token = Some("proj-token".into());
        cfg.channel_token = Some("chan-token".into());
        let c = Client::with_transport(cfg, script.clone() as Arc<dyn Transport>).unwrap();
        (c, script)
    };
    c.project().unwrap();
    let headers = &last(&script).headers;
    assert!(headers
        .iter()
        .any(|(k, v)| k.eq_ignore_ascii_case("authorization") && v == "Bearer proj-token"));
    assert!(headers
        .iter()
        .any(|(k, v)| k.eq_ignore_ascii_case("x-project-token") && v == "proj-token"));
    assert!(headers
        .iter()
        .any(|(k, v)| k.eq_ignore_ascii_case("x-channel-token") && v == "chan-token"));
}

#[test]
fn d18_direct_dependency_names() {
    let toml = include_str!("../Cargo.toml");
    let after = toml.split("[dependencies]").nth(1).unwrap();
    let section = after.split('[').next().unwrap();
    let allowed = [
        "reqwest",
        "serde",
        "serde_json",
        "sha2",
        "md-5",
        "zip",
        "ed25519-dalek",
        "rsa",
        "unicode-normalization",
    ];
    for line in section.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let name = line.split('=').next().unwrap().trim();
        assert!(
            allowed.contains(&name),
            "direct dependency {name} is not on the D18 whitelist"
        );
    }
    assert!(!section.contains("ureq"));
    assert!(!section.contains("tokio"));
    assert!(!section.contains("hyper"));
}
