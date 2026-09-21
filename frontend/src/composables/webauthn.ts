/**
 * composables/webauthn.ts
 *
 * 把 go-webauthn 返回的 JSON（challenge / user.id 为 base64url 字符串）
 * 转成 WebAuthn API 需要的 ArrayBuffer，再把断言/证明编回 JSON 交给后端。
 */

function b64urlToBytes (input: string): ArrayBuffer {
  const pad = '='.repeat((4 - (input.length % 4)) % 4)
  const b64 = input.replace(/-/g, '+').replace(/_/g, '/') + pad
  const bin = atob(b64)
  return Uint8Array.from(bin, ch => ch.codePointAt(0) ?? 0).buffer
}

function bytesToB64url (buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf)
  const bin = Array.from(bytes, byte => String.fromCodePoint(byte)).join('')
  return btoa(bin).replaceAll('+', '-').replaceAll('/', '_').replaceAll('=', '')
}

function asRecord (value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object') {
    throw new Error('invalid webauthn payload')
  }
  return value as Record<string, unknown>
}

function unwrapPublicKey (raw: unknown): Record<string, unknown> {
  let value: unknown = raw
  if (typeof value === 'string') {
    value = JSON.parse(value)
  }
  const obj = asRecord(value)
  if (obj.publicKey && typeof obj.publicKey === 'object') {
    return asRecord(obj.publicKey)
  }
  return obj
}

function decodeDescriptor (cred: unknown): PublicKeyCredentialDescriptor {
  const row = asRecord(cred)
  const id = row.id
  return {
    type: (row.type as PublicKeyCredentialType | undefined) ?? 'public-key',
    id: typeof id === 'string' ? b64urlToBytes(id) : id as BufferSource,
    transports: row.transports as AuthenticatorTransport[] | undefined,
  }
}

function serializeCredential (cred: PublicKeyCredential): Record<string, unknown> {
  const out: Record<string, unknown> = {
    id: cred.id,
    rawId: bytesToB64url(cred.rawId),
    type: cred.type,
    clientExtensionResults: cred.getClientExtensionResults(),
  }
  if (cred.authenticatorAttachment) {
    out.authenticatorAttachment = cred.authenticatorAttachment
  }
  const att = cred.response as AuthenticatorAttestationResponse
  const assertion = cred.response as AuthenticatorAssertionResponse
  const response: Record<string, unknown> = {
    clientDataJSON: bytesToB64url(cred.response.clientDataJSON),
  }
  if ('attestationObject' in att && att.attestationObject) {
    response.attestationObject = bytesToB64url(att.attestationObject)
    if (typeof att.getTransports === 'function') {
      response.transports = att.getTransports()
    }
  }
  if ('authenticatorData' in assertion && assertion.authenticatorData) {
    response.authenticatorData = bytesToB64url(assertion.authenticatorData)
    response.signature = bytesToB64url(assertion.signature)
    if (assertion.userHandle) {
      response.userHandle = bytesToB64url(assertion.userHandle)
    }
  }
  out.response = response
  return out
}

/** 注册新 Passkey（create）。 */
export async function webauthnCreate (options: unknown): Promise<Record<string, unknown>> {
  const pk = unwrapPublicKey(options)
  const user = asRecord(pk.user)
  const cred = await navigator.credentials.create({
    publicKey: {
      ...pk,
      challenge: b64urlToBytes(String(pk.challenge)),
      user: {
        ...user,
        name: String(user.name ?? ''),
        displayName: String(user.displayName ?? ''),
        id: typeof user.id === 'string' ? b64urlToBytes(user.id) : user.id as BufferSource,
      },
      excludeCredentials: Array.isArray(pk.excludeCredentials)
        ? pk.excludeCredentials.map(item => decodeDescriptor(item))
        : undefined,
    } as PublicKeyCredentialCreationOptions,
  })
  if (!cred || cred.type !== 'public-key') {
    throw new Error('webauthn create cancelled')
  }
  return serializeCredential(cred as PublicKeyCredential)
}

/** 用已有 Passkey 断言（get）。 */
export async function webauthnGet (options: unknown): Promise<Record<string, unknown>> {
  const pk = unwrapPublicKey(options)
  const cred = await navigator.credentials.get({
    publicKey: {
      ...pk,
      challenge: b64urlToBytes(String(pk.challenge)),
      allowCredentials: Array.isArray(pk.allowCredentials)
        ? pk.allowCredentials.map(item => decodeDescriptor(item))
        : undefined,
    } as PublicKeyCredentialRequestOptions,
  })
  if (!cred || cred.type !== 'public-key') {
    throw new Error('webauthn get cancelled')
  }
  return serializeCredential(cred as PublicKeyCredential)
}
