//! Live client-plane integration. Does not stop Docker or the KiriVers process.
//!
//! Fixture: `D:\KiriVers\configs\sdk-fixture.json` (or `KIRIVERS_SDK_FIXTURE`).
//! Unique `device_id`: `sdk-rust-<pid>-<nanos>`.

use std::path::PathBuf;
use std::time::{SystemTime, UNIX_EPOCH};

use kirivers_client::*;
use serde_json::Value;

const TARGET_SHA: &str = "7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969";

fn fixture_path() -> PathBuf {
    if let Ok(p) = std::env::var("KIRIVERS_SDK_FIXTURE") {
        return PathBuf::from(p);
    }
    PathBuf::from(r"D:\KiriVers\configs\sdk-fixture.json")
}

fn write_backend_issue(repro: &str, expected: &str, actual: &str) {
    let body = format!(
        "# Backend issue (Rust SDK integration)\n\n\
         Live client plane: `http://127.0.0.1:8080`  \n\
         Fixture: `D:\\KiriVers\\configs\\sdk-fixture.json`\n\n\
         The client plane is up (`GET /api/v1/health` → 200). This worktree did **not** edit server code.\n\
         The Rust SDK parsed the error envelope correctly. Mock-Transport unit/contract tests pass.\n\n\
         ## Repro\n\n{repro}\n\n\
         ## Expected\n\n{expected}\n\n\
         ## Actual\n\n{actual}\n\n\
         ## Suggested fix\n\n\
         Re-seed the local fixture (for example `.trellis/tasks/09-17-client-sdk/scripts/seed_local_fixture.py`) \
         so slug `sdk-fixture` has published `1.0.0` and `1.1.0` `windows/x86_64` stable artifacts. \
         Confirm `require_client_token` still matches the fixture (`false`).\n"
    );
    let _ = std::fs::write(
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("BACKEND_ISSUE.md"),
        body,
    );
}

#[test]
fn check_download_sha256_against_local_client_plane() {
    let path = fixture_path();
    let raw = match std::fs::read_to_string(&path) {
        Ok(s) => s,
        Err(e) => panic!("sdk-fixture.json not readable at {}: {e}", path.display()),
    };
    let fix: Value = serde_json::from_str(&raw).expect("fixture json");
    let base = fix["client_base_url"]
        .as_str()
        .unwrap_or("http://127.0.0.1:8080");
    let project = fix["project_ref"].as_str().unwrap_or("sdk-fixture");
    let channel = fix["channel"].as_str().unwrap_or("stable");
    let os = fix["os"].as_str().unwrap_or("windows");
    let arch = fix["arch"].as_str().unwrap_or("x86_64");
    let current = fix["current_version"].as_str().unwrap_or("1.0.0");
    let target = fix["target_version"].as_str().unwrap_or("1.1.0");
    let want_sha = fix["sha256"]["1.1.0"].as_str().unwrap_or(TARGET_SHA);

    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let device_id = format!("sdk-rust-{}-{nanos}", std::process::id());

    let client = match Client::new(Config::new(base, project)) {
        Ok(c) => c,
        Err(e) => panic!("client construct: {e}"),
    };

    match client.health() {
        Ok(_) => {}
        Err(Error::Transport(e)) => {
            eprintln!("skipping live integration: client plane not reachable at {base} ({e})");
            return;
        }
        Err(e) => panic!(
            "client plane at {base} returned {e}; this agent must not start or stop the server"
        ),
    }

    let outcome = match client.check(&CheckRequest {
        current_version: current.into(),
        os: os.into(),
        arch: arch.into(),
        channel: Some(channel.into()),
        device_id: Some(device_id.clone()),
        ..CheckRequest::default()
    }) {
        Ok(o) => o,
        Err(e) => {
            write_backend_issue(
                &format!(
                    "1. `GET {base}/api/v1/projects/{project}`\n\
                     2. `POST {base}/api/v1/projects/{project}/update/check` from {current} \
                     (unique `device_id` `sdk-rust-<random>`; not logged by SDK)"
                ),
                "Project exists. HTTP 200 UpdateCheck with version_semver 1.1.0 and catalog sha256",
                &format!("{e}"),
            );
            panic!("check failed (BACKEND_ISSUE.md written): {e}");
        }
    };

    let body = match outcome {
        CheckOutcome::Update { body, .. } => body,
        other => {
            write_backend_issue(
                "POST /update/check current_version=1.0.0 os=windows arch=x86_64 channel=stable",
                "200 has_update with version_semver=1.1.0",
                &format!("{other:?}"),
            );
            panic!("expected update, got {other:?}");
        }
    };

    if body.version_semver.as_deref() != Some(target) {
        write_backend_issue(
            "check 1.0.0 → expected target 1.1.0",
            target,
            &format!("{:?}", body.version_semver),
        );
        panic!("unexpected target version");
    }
    if !body.sha256.eq_ignore_ascii_case(want_sha) {
        write_backend_issue("check catalog sha256 for 1.1.0", want_sha, &body.sha256);
        panic!("unexpected catalog sha256");
    }

    let dl = match client.download(&body.package_url, None) {
        Ok(d) => d,
        Err(e) => {
            write_backend_issue(
                &format!("GET {}", body.package_url),
                "200 package bytes",
                &format!("{e}"),
            );
            panic!("download failed: {e}");
        }
    };
    let actual = StdHasher.sha256_hex(&dl.body);
    if !actual.eq_ignore_ascii_case(want_sha) {
        write_backend_issue(
            &format!("SHA-256 of GET {}", body.package_url),
            want_sha,
            &actual,
        );
        panic!("downloaded hash mismatch");
    }

    let _ = device_id; // uniqueness only; never printed by the SDK
}
