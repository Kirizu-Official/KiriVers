"""Live client-plane integration. Does not stop Docker or the KiriVers process."""

from __future__ import annotations

import hashlib
import json
import secrets
import textwrap
from pathlib import Path

import pytest

from kirivers_client import Client, Config, Updater

FIXTURE = Path(r"D:\KiriVers\configs\sdk-fixture.json")
BACKEND_ISSUE = Path(__file__).resolve().parents[1] / "BACKEND_ISSUE.md"


def _write_backend_issue(*, repro: str, expected: str, actual: str, suggested: str) -> None:
    BACKEND_ISSUE.write_text(
        textwrap.dedent(
            f"""\
            # Backend issue (Python SDK integration)

            ## Repro
            {repro}

            ## Expected
            {expected}

            ## Actual
            {actual}

            ## Suggested fix
            {suggested}
            """
        ),
        encoding="utf-8",
    )


def test_check_download_sha256_against_local_client_plane(tmp_path: Path):
    if not FIXTURE.is_file():
        pytest.fail(f"sdk-fixture.json missing at {FIXTURE}")
    fixture = json.loads(FIXTURE.read_text(encoding="utf-8"))
    base = fixture["client_base_url"]
    project = fixture["project_ref"]
    channel = fixture["channel"]
    os_name = fixture["os"]
    arch = fixture["arch"]
    current = fixture["current_version"]
    target = fixture["target_version"]
    expected_sha = fixture["sha256"][target]
    device_id = f"sdk-python-{secrets.token_hex(8)}"

    client = Client(Config(base_url=base, project_ref=project))
    try:
        try:
            health = client.health()
        except Exception as exc:  # noqa: BLE001
            _write_backend_issue(
                repro="GET http://127.0.0.1:8080/api/v1/health from kirivers-client",
                expected="HTTP 200 with JSON status/ready",
                actual=repr(exc),
                suggested="Keep the host client plane listening on :8080; do not treat Docker Compose as the client API.",
            )
            raise
        assert health.status, health

        check = client.check(
            current_version=current,
            os=os_name,
            arch=arch,
            channel=channel,
            device_id=device_id,
        )
        if check.no_update or check.not_modified or check.update is None:
            _write_backend_issue(
                repro=f"POST /api/v1/projects/{project}/update/check current_version={current} os={os_name} arch={arch}",
                expected=f"HTTP 200 has_update toward {target} sha256={expected_sha}",
                actual=f"status={check.status} no_update={check.no_update} not_modified={check.not_modified}",
                suggested="Re-seed configs/sdk-fixture.json so 1.0.0 → 1.1.0 is a published single_file line.",
            )
            pytest.fail("client plane returned no update for sdk-fixture 1.0.0")

        update = check.update
        if (update.version_semver or "") != target:
            _write_backend_issue(
                repro="sdk-fixture check from 1.0.0",
                expected=f"version_semver={target}",
                actual=f"version_semver={update.version_semver!r} reason={update.reason!r}",
                suggested="Publish 1.1.0 for windows/x86_64 on channel stable.",
            )
            pytest.fail(f"unexpected target {update.version_semver}")

        dest = tmp_path / (update.file_name or "pkg.bin")
        updater = Updater(client, archive_unpacker=None, file_store=None, replacer=None)
        result = updater.run(
            current_version=current,
            os=os_name,
            arch=arch,
            channel=channel,
            device_id=device_id,
            dest_path=dest,
            apply=False,
        )
        data = Path(result.staged_path or dest).read_bytes()
        digest = hashlib.sha256(data).hexdigest()
        if digest != expected_sha:
            _write_backend_issue(
                repro=f"download package_url={update.package_url}",
                expected=expected_sha,
                actual=digest,
                suggested="Object bytes for 1.1.0 must match configs/sdk-fixture.json sha256.1.1.0.",
            )
            pytest.fail(f"sha256 mismatch: {digest}")
        assert result.sha256 == expected_sha
        assert len(data) == fixture["check_sample"]["size"]
    finally:
        client.close()
