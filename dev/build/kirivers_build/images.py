"""Docker Hub publication: one native per-arch image, then the multi-arch manifest.

Both subcommands are release-job only, and neither is used by this CLI's other
commands, so stdout stays empty here as everywhere else (C12) and the image tag
grammar is an external contract: `<semver>_<arch>` per architecture with no `v`
prefix, `<semver>` and `latest` for the manifest (C10).
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
from pathlib import Path

from . import build, common

IMAGE = "kirizuofficial/kirivers"
BUILDER = "kirivers-release"
IMAGE_ARCHES = ("amd64", "arm64")
DEFAULT_ARCHES = "amd64 arm64"
MISSING_SECRET = "missing Docker Hub secret DOCKERHUB_USERNAME and/or DOCKERHUB_TOKEN"
# Kept out of every later child so the token cannot leak into buildx argv/env; it
# is still present while `docker login` runs, exactly as in the shell original.
TOKEN_ENV = "DOCKERHUB_TOKEN"


def _docker(
    args: list[str],
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
    stdin_text: str | None = None,
    quiet: bool = False,
) -> subprocess.CompletedProcess:
    """Run docker.

    A probe (`quiet=True`) discards both streams because the shell original ran it
    with `>/dev/null 2>&1` and the verdict is all the caller needs; anything else
    keeps docker's own progress but moves stdout onto our stderr so no contract
    stream is ever polluted (C12, defect D1). A missing docker binary is reported
    like a failing one, which is how `set -e` plus `if !` handled it.
    """
    try:
        return subprocess.run(
            ["docker", *args],
            cwd=str(cwd) if cwd else None,
            env=env,
            input=stdin_text,
            stdout=subprocess.DEVNULL if quiet else sys.stderr,
            stderr=subprocess.DEVNULL if quiet else None,
            text=True,
            check=False,
        )
    except OSError as exc:
        return subprocess.CompletedProcess(["docker", *args], 127, "", str(exc))


def _credentials() -> "tuple[str, dict[str, str]] | None":
    """Validate the two Hub secrets and prepare the token-free child env."""
    username = os.environ.get("DOCKERHUB_USERNAME") or ""
    token = os.environ.get(TOKEN_ENV) or ""
    if not username or not token:
        common.log(MISSING_SECRET)
        return None
    child_env = dict(os.environ)
    child_env.pop(TOKEN_ENV, None)
    return token, child_env


def _require_buildx() -> int:
    """0 when buildx works, 1 with the original message (docker may be absent)."""
    if _docker(["buildx", "version"], quiet=True).returncode != 0:
        common.log("docker buildx is required")
        return common.FAIL_EXIT
    return 0


def _login(token: str, child_env: "dict[str, str]") -> int:
    """Authenticate with the token on stdin, never on the command line.

    Not quiet: docker's own failure reason (bad credentials, no daemon) has to stay
    readable on stderr, just like the shell original that only muted stdout.
    """
    return _docker(
        ["login", "--username", os.environ.get("DOCKERHUB_USERNAME") or "", "--password-stdin"],
        env=child_env,
        stdin_text=token + "\n",  # `echo "$TOKEN" |` in the original, newline included
    ).returncode


def docker_image(argv: list[str]) -> int:
    """Build and push ONE linux arch from a musl binary this host made natively.

    Every guard here runs before docker is touched: argument shape, the two
    secrets, the no-QEMU host-arch invariant and the musl assertion.
    """
    arch = argv[0] if argv else ""
    version = argv[1] if len(argv) > 1 else ""
    if arch not in IMAGE_ARCHES or not version:
        common.log("usage: kirivers.py docker-image <amd64|arm64> <semver>")
        return common.USAGE_EXIT
    secrets = _credentials()
    if secrets is None:
        return common.FAIL_EXIT
    token, child_env = secrets

    host_arch = common.host_goarch()
    if host_arch != arch:
        # The no-QEMU invariant: an emulated build would silently ship a binary
        # the release job never compiled on that architecture.
        common.log(
            f"docker-image: linux/{arch} must be built on an {arch} runner (host is {host_arch})"
        )
        return common.FAIL_EXIT

    bin_name = common.asset_name("linux", arch)
    # The Dockerfile's USE_PREBUILT stage COPYs this name from the build context,
    # i.e. the repository root; the release job stages it under dist/ first.
    binary = common.ROOT / bin_name
    if not binary.is_file() and (common.ROOT / "dist" / bin_name).is_file():
        shutil.copy2(common.ROOT / "dist" / bin_name, binary)
    if not binary.is_file():
        common.log(f"missing {bin_name} (linux musl artifact)")
        return common.FAIL_EXIT

    status = _assert_musl(binary)
    if status:
        return status
    if _require_buildx():
        return common.FAIL_EXIT
    status = _login(token, child_env)
    if status:
        return status

    if _docker(["buildx", "inspect", BUILDER], cwd=common.ROOT, env=child_env, quiet=True).returncode:
        status = _docker(
            ["buildx", "create", "--name", BUILDER, "--driver", "docker-container", "--use"],
            cwd=common.ROOT,
            env=child_env,
        ).returncode
    else:
        status = _docker(["buildx", "use", BUILDER], cwd=common.ROOT, env=child_env).returncode
    if status:
        return status

    tag = f"{IMAGE}:{version}_{arch}"
    common.log(f"docker build --platform linux/{arch} -> {tag}")
    return _docker(
        [
            "buildx",
            "build",
            "--platform",
            f"linux/{arch}",
            "--build-arg",
            "USE_PREBUILT=1",
            "-f",
            "dev/build/Dockerfile",
            "--target",
            "runtime",
            "-t",
            tag,
            "--push",
            ".",
        ],
        cwd=common.ROOT,
        env=child_env,
    ).returncode


def _assert_musl(binary: Path) -> int:
    """Reuse build.py's musl guard instead of exec'ing the retired shell script.

    An unverified binary must never enter an Alpine image, so this is the same
    fail-closed gate `docker-image.sh` shelled out to; build.py has no dependency
    on this module, so the import is one-way.
    """
    return build.assert_linux_musl([str(binary)])


def docker_manifest(argv: list[str]) -> int:
    """Compose `:latest` and `:<semver>` from the per-arch tags; registry-side only."""
    version = argv[0] if argv else ""
    if not version:
        common.log("usage: kirivers.py docker-manifest <semver> [arch...]")
        return common.USAGE_EXIT
    # Mirrors `archs=${*:-amd64 arm64}` plus the IFS word split: a whitespace-only
    # arch list falls back to both arches instead of yielding empty arguments.
    archs = (" ".join(argv[1:])).split() or DEFAULT_ARCHES.split()

    # The shell original checked the secrets and buildx before validating the arch
    # names, so an unsupported arch on an unconfigured host reports the missing
    # secret first. Preserved: exit codes here decide what a run looks like.
    secrets = _credentials()
    if secrets is None:
        return common.FAIL_EXIT
    token, child_env = secrets
    if _require_buildx():
        return common.FAIL_EXIT

    refs: list[str] = []
    for arch in archs:
        if arch not in IMAGE_ARCHES:
            common.log(f"docker-manifest: only amd64/arm64 are published (got {arch})")
            return common.USAGE_EXIT
        refs.append(f"{IMAGE}:{version}_{arch}")

    status = _login(token, child_env)
    if status:
        return status

    if not refs:
        # Unreachable through the default arch list; kept as the shell's guard.
        common.log("docker-manifest: no architecture refs to compose")
        return common.USAGE_EXIT
    for ref in refs:
        # Refuse to retag :latest unless every per-arch tag really exists;
        # imagetools would otherwise publish a manifest pointing at a missing digest.
        if _docker(["buildx", "imagetools", "inspect", ref], env=child_env, quiet=True).returncode:
            common.log(f"docker-manifest: {ref} is not in the registry")
            return common.FAIL_EXIT

    common.log(f"docker buildx imagetools create -> {IMAGE}:latest {IMAGE}:{version}")
    return _docker(
        ["buildx", "imagetools", "create", "-t", f"{IMAGE}:latest", "-t", f"{IMAGE}:{version}", *refs],
        env=child_env,
    ).returncode
