---
title: "Tokens"
description: "Project tokens and CI tokens are separate. Plaintext appears only on create."
---

# Tokens

Nav **Tokens**. Tabs **Project tokens** / **CI tokens**. Store token lives on [Settings](./settings) **Security**. Channel token lives on [Channels](./channels).

1. Open **Tokens**, pick **Project tokens** or **CI tokens**.
2. Click **Create token**. Fill **Name**, **Scopes**, optional **Expires at (empty = never)**.
3. Dialog **Save your token now**: plaintext is shown once; afterwards only the **Fingerprint**.
4. **Revoke token** invalidates callers immediately. Missing session → 401 <ErrorCode code="UNAUTHORIZED" />.

CI tokens call admin-plane `ci/releases`; see [CI releases](/en/api/ci). Concepts: [Access tokens](/en/guide/features/tokens).
