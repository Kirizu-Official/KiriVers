use ed25519_dalek::pkcs8::DecodePublicKey as EdDecodePublicKey;
use ed25519_dalek::{Signature as EdSignature, Verifier, VerifyingKey};
use rsa::pkcs1v15::{Signature as RsaSignature, VerifyingKey as RsaVerifyingKey};
use rsa::pkcs8::DecodePublicKey as RsaDecodePublicKey;
use rsa::signature::Verifier as RsaVerifier;
use rsa::RsaPublicKey;
use sha2::Sha256;

use crate::error::Error;

/// Build the check / integrity / diff signature payload (`pkg/signature.BuildCheckPayload`).
/// Empty fields stay empty strings so position is stable:
/// `integer \n semver \n root_hash \n package_url \n size \n sha256`.
pub fn build_check_payload(
    version_integer: &str,
    version_semver: &str,
    root_hash: &str,
    package_url: &str,
    size: &str,
    sha256_hex: &str,
) -> String {
    [
        version_integer,
        version_semver,
        root_hash,
        package_url,
        size,
        sha256_hex,
    ]
    .join("\n")
}

pub fn payload_from_parts(
    version_integer: Option<i64>,
    version_semver: Option<&str>,
    root_hash: &str,
    package_url: &str,
    size: i64,
    sha256_hex: &str,
) -> String {
    build_check_payload(
        &version_integer.map(|n| n.to_string()).unwrap_or_default(),
        version_semver.unwrap_or(""),
        root_hash,
        package_url,
        &size.to_string(),
        sha256_hex,
    )
}

/// Verify `signature` (standard base64) over [`build_check_payload`].
pub trait SignatureVerifier: Send + Sync {
    fn verify(
        &self,
        algo: &str,
        public_key_pem: &str,
        payload: &str,
        sig_b64: &str,
    ) -> Result<(), Error>;
}

/// Default verifier: `ed25519-dalek` + `rsa` (PKCS1v15 SHA-256).
#[derive(Debug, Default, Clone, Copy)]
pub struct StdSignatureVerifier;

impl SignatureVerifier for StdSignatureVerifier {
    fn verify(
        &self,
        algo: &str,
        public_key_pem: &str,
        payload: &str,
        sig_b64: &str,
    ) -> Result<(), Error> {
        let sig = b64_decode(sig_b64.trim()).map_err(Error::Signature)?;
        match algo {
            "ed25519" => {
                let key = <VerifyingKey as EdDecodePublicKey>::from_public_key_pem(public_key_pem)
                    .map_err(|e| Error::Signature(format!("ed25519 public key: {e}")))?;
                let signature = EdSignature::from_slice(&sig)
                    .map_err(|e| Error::Signature(format!("ed25519 signature: {e}")))?;
                Verifier::verify(&key, payload.as_bytes(), &signature)
                    .map_err(|_| Error::Signature("ed25519 mismatch".into()))
            }
            "rsa-sha256" => {
                let key = <RsaPublicKey as RsaDecodePublicKey>::from_public_key_pem(public_key_pem)
                    .map_err(|e| Error::Signature(format!("rsa public key: {e}")))?;
                let vk = RsaVerifyingKey::<Sha256>::new(key);
                let signature = RsaSignature::try_from(sig.as_slice())
                    .map_err(|e| Error::Signature(format!("rsa signature: {e}")))?;
                RsaVerifier::verify(&vk, payload.as_bytes(), &signature)
                    .map_err(|_| Error::Signature("rsa-sha256 mismatch".into()))
            }
            other => Err(Error::Signature(format!("unsupported algorithm {other}"))),
        }
    }
}

/// Standard base64 (with padding). Kept in-tree so we do not add a second codec crate.
pub fn b64_decode(input: &str) -> Result<Vec<u8>, String> {
    fn val(c: u8) -> Option<u8> {
        match c {
            b'A'..=b'Z' => Some(c - b'A'),
            b'a'..=b'z' => Some(c - b'a' + 26),
            b'0'..=b'9' => Some(c - b'0' + 52),
            b'+' => Some(62),
            b'/' => Some(63),
            _ => None,
        }
    }
    let bytes: Vec<u8> = input.bytes().filter(|b| !b.is_ascii_whitespace()).collect();
    if bytes.len() % 4 != 0 {
        return Err("invalid base64 length".into());
    }
    let mut out = Vec::with_capacity(bytes.len() / 4 * 3);
    for chunk in bytes.chunks(4) {
        let pad = chunk.iter().filter(|&&c| c == b'=').count();
        let mut n = 0u32;
        for (i, c) in chunk.iter().enumerate() {
            if *c == b'=' {
                if i < 2 {
                    return Err("invalid base64 padding".into());
                }
                continue;
            }
            let v = val(*c).ok_or_else(|| "invalid base64 character".to_string())?;
            n |= (v as u32) << (18 - i * 6);
        }
        out.push((n >> 16) as u8);
        if pad < 2 {
            out.push((n >> 8) as u8);
        }
        if pad < 1 {
            out.push(n as u8);
        }
    }
    Ok(out)
}

#[cfg(test)]
pub fn b64_encode(data: &[u8]) -> String {
    const T: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::new();
    let mut i = 0;
    while i < data.len() {
        let b0 = data[i];
        let b1 = if i + 1 < data.len() { data[i + 1] } else { 0 };
        let b2 = if i + 2 < data.len() { data[i + 2] } else { 0 };
        let n = ((b0 as u32) << 16) | ((b1 as u32) << 8) | (b2 as u32);
        out.push(T[((n >> 18) & 63) as usize] as char);
        out.push(T[((n >> 12) & 63) as usize] as char);
        if i + 1 < data.len() {
            out.push(T[((n >> 6) & 63) as usize] as char);
        } else {
            out.push('=');
        }
        if i + 2 < data.len() {
            out.push(T[(n & 63) as usize] as char);
        } else {
            out.push('=');
        }
        i += 3;
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use ed25519_dalek::pkcs8::EncodePublicKey;
    use ed25519_dalek::Signer;
    use ed25519_dalek::SigningKey;

    #[test]
    fn payload_joins_six_fields() {
        assert_eq!(
            build_check_payload("1", "1.0.0", "rh", "/pkg", "12", "ab"),
            "1\n1.0.0\nrh\n/pkg\n12\nab"
        );
        assert_eq!(build_check_payload("", "", "", "", "", ""), "\n\n\n\n\n");
    }

    #[test]
    fn ed25519_roundtrip() {
        let sk = SigningKey::from_bytes(&[7u8; 32]);
        let vk = sk.verifying_key();
        let pem = vk
            .to_public_key_pem(ed25519_dalek::pkcs8::spki::der::pem::LineEnding::LF)
            .expect("pem");
        let payload = build_check_payload("", "1.1.0", "", "/p", "51", "dead");
        let sig = sk.sign(payload.as_bytes());
        let b64 = b64_encode(&sig.to_bytes());
        StdSignatureVerifier
            .verify("ed25519", &pem, &payload, &b64)
            .unwrap();
        StdSignatureVerifier
            .verify("ed25519", &pem, "tampered", &b64)
            .unwrap_err();
    }

    #[test]
    fn rsa_sha256_verifies_go_pkcs1v15_vector() {
        // PKIX public key + PKCS1v15-SHA256 signature produced with crypto/rsa
        // (same as pkg/signature.VerifyPayload). Payload is BuildCheckPayload.
        let pem = "-----BEGIN PUBLIC KEY-----\n\
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAmgDXlH9b9VT/k9FedzHX\n\
8TKZTwpTDT0sbHHW3179XDfBX2IRgfGxqWIcySkZ3S5/NpX1DlsJhzZrD3jotgqj\n\
O5yXQbfwGmFYYgA2rOBv0EofLoG/haJE8/drZYtHWfmJ9iJj+BE4Wp7VctyaCO1a\n\
B5oKXUffMc3WNEET0zNMgSDxsWFJ/P96xhDKezOlsLpzAhA5yOmD3QO+vysrYf3I\n\
KqtdRhe0qPAwB4IDJTMX1/yuWaaJz+xIyUgTINwjypnJavtUVdoLo8yfKnKJ3zu5\n\
kmZunGFuDC3cWsasLZv25ezF0elwFp4v4uBgDH+6hKwVQjol2TgovnuNb66y9VB/\n\
QQIDAQAB\n\
-----END PUBLIC KEY-----\n";
        let sig = "RsecRB6RNT6DOnFp/MiJ775eToV0VpzBjnknG93SR2prEtYfHzWW/+squa4XUkk/GaaEwgUehGn91rqOtRp6QYC8Y7psR12xocfvYfKWMO7ddrnbXB+iG9OaFg6tzylqlewrNWbmrYJ/axGIECD0nWP5mXT5+kWtalgH68TObI3upSDGPhfkeD7/w+7epYUP192yfzUGcEcG9qmLsotAt3xfwRVYJudSjduY7PB026CqzGFVMzEXUG8o1LxAMEQSDge+eYNBRFWE3BzRtRaiw2bXHTVBkfc7dwzHVYFdmTXbkFQiuI5BkLf8Dvp8I9wXRgMAICarhMnPw068P4c2jg==";
        let payload = "1\n1.1.0\n\n/p\n12\nab";
        StdSignatureVerifier
            .verify("rsa-sha256", pem, payload, sig)
            .unwrap();
        StdSignatureVerifier
            .verify("rsa-sha256", pem, "tampered", sig)
            .unwrap_err();
    }
}
