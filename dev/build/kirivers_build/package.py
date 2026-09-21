"""Release asset naming and packaging.

`asset-names` is the machine-readable statement of the naming contract (C4/C10):
release.yml loops over its output to prove every archive got uploaded, and
check-workflows counts its lines.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import zipfile
from pathlib import Path

from . import common
from .common import MATRIX, USAGE_EXIT, archive_name, asset_name

FRONTEND_ARCHIVE = "frontend-dist.zip"
SUMS_FILE = "SHA256SUMS.txt"


def _parse_os_filter(argv: list[str]) -> tuple[str, str]:
    """Split `asset-names` args into mode and OS filter; flags are order-free."""
    mode, goos_filter = "archives", ""
    for arg in argv:
        if arg == "--binaries":
            mode = "binaries"
        elif arg in {"linux", "darwin", "windows"}:
            goos_filter = arg
        else:
            common.log("usage: kirivers.py asset-names [--binaries] [linux|darwin|windows]")
            raise SystemExit(USAGE_EXIT)
    return mode, goos_filter


def asset_names(argv: list[str]) -> int:
    mode, goos_filter = _parse_os_filter(argv)
    rows = [row for row in MATRIX if not goos_filter or row[0] == goos_filter]
    if mode == "binaries":
        for row in rows:
            print(asset_name(row[0], row[1]))
        return 0
    for row in rows:
        print(archive_name(row[0], row[1]))
    if not goos_filter:
        print(FRONTEND_ARCHIVE)
        print(SUMS_FILE)
    return 0


def _run_zip(command: list[str], cwd: Path) -> None:
    """Archive with the platform zip when present (what the release job uses).

    Kept over a pure-Python writer so published archive bytes do not shift under
    a refactor that was meant to change nothing; child stdout is diverted because
    this CLI's stdout is reserved for contract lines.
    """
    status = subprocess.run(command, cwd=cwd, stdout=sys.stderr, check=False).returncode
    if status:
        raise SystemExit(status)


def _zip_one(srcdir: Path, member: str, dest: Path) -> None:
    """One binary at the archive root, matching the published per-target zip."""
    dest.unlink(missing_ok=True)
    if shutil.which("zip"):
        _run_zip(["zip", "-q", "-X", str(dest), member], srcdir)
        return
    with zipfile.ZipFile(dest, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.write(srcdir / member, member)


def _zip_tree(srcdir: Path, dest: Path) -> None:
    """Frontend dist unpacked at the archive root (no wrapper directory)."""
    dest.unlink(missing_ok=True)
    if shutil.which("zip"):
        _run_zip(["zip", "-q", "-r", "-X", str(dest), "."], srcdir)
        return
    with zipfile.ZipFile(dest, "w", zipfile.ZIP_DEFLATED) as archive:
        for root, dirs, files in os.walk(srcdir):
            dirs.sort()
            files.sort()
            for name in files:
                path = Path(root) / name
                archive.write(path, path.relative_to(srcdir).as_posix())


def package_assets(argv: list[str]) -> int:
    if len(argv) != 2:
        common.log("usage: kirivers.py package-assets <srcdir> <destdir>")
        return USAGE_EXIT
    src, dst = Path(argv[0]).resolve(), Path(argv[1])
    dst.mkdir(parents=True, exist_ok=True)
    dst = dst.resolve()

    pairs: list[tuple[str, Path]] = []
    for goos, goarch, _goarm, _libc, _os_token, _arch_tag in MATRIX:
        binary = src / asset_name(goos, goarch)
        if not binary.is_file():
            common.log(f"package-assets: missing {binary}")
            return common.FAIL_EXIT
        try:
            binary.chmod(binary.stat().st_mode | 0o111)
        except OSError:
            pass
        status = common.assert_machine(goos, goarch, binary)
        if status:
            return status
        pairs.append((archive_name(goos, goarch, common.sha256_file(binary)[:6]), binary))

    if len(pairs) != 10:
        common.log(f"package-assets: expected 10 targets, got {len(pairs)}")
        return common.FAIL_EXIT
    if not (src / "frontend-dist" / "index.html").is_file():
        common.log(f"package-assets: missing {src / 'frontend-dist' / 'index.html'} (frontend build did not run?)")
        return common.FAIL_EXIT

    for archive, binary in pairs:
        _zip_one(binary.parent, binary.name, dst / archive)
        common.log(f"wrote {archive}")
    _zip_tree(src / "frontend-dist", dst / FRONTEND_ARCHIVE)

    # Two sections: the first is verifiable with `sha256sum -c`, the second
    # documents what went inside each archive as `#` comment rows. Two-space
    # separators are the coreutils text-mode shape; shelling out on MSYS would
    # have written ` *` instead (C9).
    lines = ["# KiriVers checksums — verify the uploaded assets with: sha256sum -c SHA256SUMS.txt"]
    for archive, _binary in pairs:
        lines.append(f"{common.sha256_file(dst / archive)}  {archive}")
    lines.append(f"{common.sha256_file(dst / FRONTEND_ARCHIVE)}  {FRONTEND_ARCHIVE}")
    lines.append("# Binaries inside each archive (extract first; # keeps them out of -c)")
    for archive, binary in pairs:
        lines.append(f"# {common.sha256_file(binary)}  {binary.name} (inside {archive})")
    common.write_lf(dst / SUMS_FILE, "\n".join(lines) + "\n")

    common.log(f"package-assets: 10 archives + {FRONTEND_ARCHIVE} + {SUMS_FILE} in {dst}")
    return 0
