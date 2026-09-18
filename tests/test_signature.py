from __future__ import annotations

import base64

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ed25519, padding, rsa

from kirivers_client.adapters import CryptographyVerifier, build_check_payload, payload_from_check_fields
from kirivers_client.delta import MAGIC_BSDIFF40, MAGIC_HDIFF13, MAGIC_KVDIFFHP1, MAGIC_VCDIFF, detect_delta_algo
from kirivers_client.errors import PatchError, SignatureError
import pytest


def test_payload_format_matches_server():
    got = build_check_payload("102", "1.2.3", "roothash", "/pkg", "123456", "abcd")
    assert got == "102\n1.2.3\nroothash\n/pkg\n123456\nabcd"
    got = build_check_payload("", "1.2.3", "", "/pkg", "", "abcd")
    assert got == "\n1.2.3\n\n/pkg\n\nabcd"


def test_payload_from_check_fields_empty_integer():
    got = payload_from_check_fields(
        version_integer=None,
        version_semver="1.1.0",
        root_hash="",
        package_url="/u",
        size=51,
        sha256_hex="aa",
    )
    assert got == "\n1.1.0\n\n/u\n51\naa"


def test_ed25519_and_rsa_verify():
    verifier = CryptographyVerifier()
    payload = build_check_payload("102", "1.2.3", "root", "/u", "1", "aa")

    ed = ed25519.Ed25519PrivateKey.generate()
    ed_pub = ed.public_key().public_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PublicFormat.SubjectPublicKeyInfo,
    ).decode("ascii")
    ed_sig = base64.b64encode(ed.sign(payload.encode("utf-8"))).decode("ascii")
    verifier.verify("ed25519", ed_pub, payload, ed_sig)
    with pytest.raises(SignatureError):
        verifier.verify("ed25519", ed_pub, payload + "x", ed_sig)

    rsa_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    rsa_pub = rsa_key.public_key().public_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PublicFormat.SubjectPublicKeyInfo,
    ).decode("ascii")
    rsa_sig = base64.b64encode(
        rsa_key.sign(payload.encode("utf-8"), padding.PKCS1v15(), hashes.SHA256())
    ).decode("ascii")
    verifier.verify("rsa-sha256", rsa_pub, payload, rsa_sig)


def test_unknown_delta_magic_refuses_cross_decode():
    with pytest.raises(PatchError):
        detect_delta_algo(b"NOT-A-DELTA")
    assert detect_delta_algo(MAGIC_BSDIFF40 + b"rest") == "bsdiff"
    assert detect_delta_algo(MAGIC_HDIFF13 + b"rest") == "hdiffpatch"
    assert detect_delta_algo(MAGIC_KVDIFFHP1 + b"rest") == "hdiffpatch"
    assert detect_delta_algo(MAGIC_VCDIFF + b"rest") == "xdelta3"
