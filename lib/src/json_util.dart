/// Shared JSON coercions. Wire names stay snake_case; Dart fields are camelCase.
int? asInt(Object? value) {
  if (value == null) {
    return null;
  }
  if (value is int) {
    return value;
  }
  if (value is num) {
    return value.toInt();
  }
  if (value is String && value.isNotEmpty) {
    return int.tryParse(value);
  }
  return null;
}

String asString(Object? value, [String fallback = '']) {
  if (value == null) {
    return fallback;
  }
  return value.toString();
}

String? asStringOrNull(Object? value) {
  if (value == null) {
    return null;
  }
  return value.toString();
}

bool asBool(Object? value, [bool fallback = false]) {
  if (value is bool) {
    return value;
  }
  if (value is String) {
    if (value == 'true') {
      return true;
    }
    if (value == 'false') {
      return false;
    }
  }
  return fallback;
}

Map<String, dynamic> asMap(Object? value) {
  if (value is Map<String, dynamic>) {
    return value;
  }
  if (value is Map) {
    return Map<String, dynamic>.from(value);
  }
  return <String, dynamic>{};
}

List<dynamic> asList(Object? value) {
  if (value is List) {
    return value;
  }
  return const [];
}

/// Case-insensitive header lookup. `package:http` lowercases keys.
String headerValue(Map<String, String> headers, String name) {
  final want = name.toLowerCase();
  for (final entry in headers.entries) {
    if (entry.key.toLowerCase() == want) {
      return entry.value;
    }
  }
  return '';
}

void putIfNotNull(Map<String, dynamic> target, String key, Object? value) {
  if (value == null) {
    return;
  }
  if (value is String && value.isEmpty) {
    return;
  }
  if (value is List && value.isEmpty) {
    return;
  }
  target[key] = value;
}
