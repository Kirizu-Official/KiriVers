"""Binary-delta container magics. Unknown magic must not be cross-decoded."""

from __future__ import annotations

from kirivers_client.errors import PatchError

# Official / fallback containers from docs/delta-engines.md.
MAGIC_KVDIFFHP1 = b"KVDIFFHP1\n"
MAGIC_HDIFF13 = b"HDIFF13&"
MAGIC_BSDIFF40 = b"BSDIFF40"
MAGIC_VCDIFF = bytes((0xD6, 0xC3, 0xC4))

ALGO_HDIFFPATCH = "hdiffpatch"
ALGO_BSDIFF = "bsdiff"
ALGO_XDELTA3 = "xdelta3"


def detect_delta_algo(delta: bytes) -> str:
    """Return the algorithm name for a delta blob, or raise PatchError."""
    if delta.startswith(MAGIC_KVDIFFHP1) or delta.startswith(MAGIC_HDIFF13):
        return ALGO_HDIFFPATCH
    if delta.startswith(MAGIC_BSDIFF40):
        return ALGO_BSDIFF
    if delta.startswith(MAGIC_VCDIFF):
        return ALGO_XDELTA3
    raise PatchError("unknown delta magic; refusing to cross-decode")
