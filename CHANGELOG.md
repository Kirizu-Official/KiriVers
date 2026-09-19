# Changelog

## 0.1.0

- First release of the official Dart client SDK (`kirivers_client`).
- Handwritten native JSON client covering the client-plane surface in
  `openapi.client.json` (revision `1.0.0` / `B443DEA6`).
- High-level `Updater` (check → download → verify → optional patch/unpack →
  optional `File.rename` replace).
- Default adapters: `package:http`, `dart:convert`, `package:crypto`,
  `package:archive`, `package:cryptography`, `unorm_dart`, `File.rename`.
- `Client.check` sends `capabilities: ["full_package"]` only. `Updater` adds
  `patch_package` (default zip); `file_list` only with a writable `FileStore`.
- `Patcher` is an injected interface; the SDK does not bundle FFI delta
  engines and does not advertise `binary_delta` unless a Patcher is provided.
