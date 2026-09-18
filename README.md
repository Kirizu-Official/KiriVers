"""KiriVers official Python client SDK

PyPI: **kirivers-client** · Python 3.10+ · branch `sdk/python` (this repository root *is* the package root).

Handwritten Client for the native JSON client plane (`openapi.client.json`). No OpenAPI codegen. Runtime dependencies are only `requests` and `cryptography`.

This is **not** a one-click installer. `os.replace` is the default `Replacer`, but a busy destination (typical for a running Windows binary) raises to the caller. Binary delta apply has **no** default: inject a `Patcher` or the updater downloads the full package.

## Install

```bash
pip install kirivers-client
```

From this branch:

```bash
pip install .
```

## Quick start

```python
from kirivers_client import Client, Config, Updater

client = Client(Config(base_url="https://client.example", project_ref="my-app"))
check = client.check(current_version="1.0.0", os="windows", arch="x86_64", channel="stable")
if check.no_update or check.not_modified:
    print("up to date")
elif check.update:
    pkg = client.download_url(check.update.package_url)
    # verify pkg.body against check.update.sha256, then stage it yourself

updater = Updater(client)
result = updater.run(
    current_version="1.0.0",
    os="windows",
    arch="x86_64",
    channel="stable",
    device_id="your-stable-device-id",  # caller-generated; SDK never invents this
    dest_path="downloads/app.bin",
    apply=False,  # download + sha256 only
)
```

Pass `device_id`, channel, `custom`, os/arch, and the current version yourself. The SDK does not create a device identity and does not write plaintext `device_id` to logs.

## Adapters and capabilities (D13 / D18)

Default `POST /update/check` from `Client.check()` sends `capabilities: ["full_package"]` only.

`Updater` derives extra bits from adapters that are actually present:

| Adapter | Default | Check field when present |
|---------|---------|--------------------------|
| `Transport` | `requests` (injectable) | `full_package` |
| `JSON` | stdlib `json` | (not an adapter) |
| `Hasher` | stdlib `hashlib` | download / integrity verify |
| `FileStore` | `pathlib` / `os` + NFC (`unicodedata`) | `file_list` when it can write individual files |
| `ArchiveUnpacker` | stdlib `zipfile` | `patch_package` |
| `SignatureVerifier` | `cryptography` (Ed25519 + RSA-SHA256) | verify `signature` over the check payload |
| `Replacer` | `os.replace` | apply only if `Updater.run(apply=True)` |
| `Patcher` | **interface only** | `binary_delta` + `accepted_delta_algos` from `Patcher.supported_algos()` |

Never send empty capability lists. `accepted_delta_algos` is omitted unless a live `Patcher` advertises algorithms. `local_sha256` is sent only on `POST /update/diff`, never on check.

To enable binary delta:

```python
class MyPatcher:
    def supported_algos(self):
        return ["bsdiff"]  # and/or hdiffpatch, xdelta3

    def apply(self, old: bytes, delta: bytes) -> bytes:
        # reject unknown magics (KVDIFFHP1 / HDIFF13& / BSDIFF40 / VCDIFF)
        ...

updater = Updater(client, patcher=MyPatcher())
```

Unknown delta magic is an error; the updater falls back to the full package and will not cross-decode.

Locked-file replace (Windows in-use EXE, APK sideload, …) is **your** `Replacer`. The default `os.replace` is not a complete installer.

| Runtime | Default Replacer | Notes |
|---------|------------------|-------|
| Windows desktop | `os.replace` | Busy destination (running EXE) raises; inject a `Replacer` that owns unlock / `MoveFileEx` |
| macOS / Linux desktop | `os.replace` | Permission and code-sign limits are the caller’s |
| Android / HarmonyOS | none | Caller starts the package installer |
| iOS | none | IPA / MDM replacement is the caller’s |

## Native JSON API (`Client`)

Covered (store feeds and leftover routes are not):

| Method | Path |
|--------|------|
| GET | `/api/v1/health` |
| GET | `/api/v1/projects/{project_ref}` |
| POST | `.../clients/report` |
| POST | `.../update/check` |
| GET | `.../changelog/{channel}/{os}/{arch}` |
| GET | `.../versions/{version}/integrity` |
| POST | `.../update/diff` |
| POST | `.../update/pack` (poll the **same** JSON; no `/pack/status`) |
| GET/HEAD | `.../packages/{ref}` (`Range`, keep `exp`/`sig`) |
| GET | `.../channels`, `.../matrix`, `.../languages` |
| GET | `.../announcements` |
| POST | `.../telemetry/report` (202; failures must not block apply) |
| GET/HEAD | `.../media/{id}` |

HTTP 204 on check is “no update”, not an error. HTTP 304 is an ETag hit (`If-None-Match`). Errors parse `{ "error": { "code", "message", "details" } }`; unknown `code` values stay opaque strings.

Inject `Transport` for tests or custom HTTP:

```python
from kirivers_client import Client, Config, TransportRequest, TransportResponse

class FakeTransport:
    def request(self, req: TransportRequest) -> TransportResponse:
        ...

client = Client(Config(base_url="http://127.0.0.1:8080", project_ref="demo"), transport=FakeTransport())
```

## OpenAPI snapshot

`openapi.client.json` plus `OPENAPI_REVISION` (`1.0.0` + `B443DEA6`) live at the package root. Contract tests load that snapshot; they do not start Docker.

## Tests

```bash
pip install -e ".[dev]"
python -m pytest
```

Integration tests talk to a host client plane at `http://127.0.0.1:8080` using `configs/sdk-fixture.json` from the KiriVers server tree. Docker Compose is only for that server’s Postgres/Redis — it is not an SDK dependency.
