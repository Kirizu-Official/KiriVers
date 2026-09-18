/// Official KiriVers native JSON client SDK for Dart 3.
///
/// Covers the client plane in `openapi.client.json`. Not a store-feed client.
/// Browser / `dart:html` is unsupported. Flutter is optional (this package
/// does not depend on the Flutter SDK).
library;

export 'src/adapters.dart';
export 'src/capabilities.dart';
export 'src/client.dart';
export 'src/config.dart';
export 'src/delta.dart';
export 'src/errors.dart';
export 'src/file_store.dart';
export 'src/hasher.dart';
export 'src/models.dart';
export 'src/paths.dart';
export 'src/pathutil.dart';
export 'src/replacer.dart';
export 'src/signature.dart';
export 'src/transport.dart';
export 'src/unpacker.dart';
export 'src/updater.dart';
