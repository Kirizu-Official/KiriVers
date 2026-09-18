#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
PROPS="$ROOT/.mvn/wrapper/maven-wrapper.properties"
DIST_URL="https://repo.maven.apache.org/maven2/org/apache/maven/apache-maven/3.9.9/apache-maven-3.9.9-bin.zip"
if [[ -f "$PROPS" ]]; then
  DIST_URL="$(grep '^distributionUrl=' "$PROPS" | cut -d= -f2- | tr -d '\r')"
fi
NAME="$(basename "$DIST_URL" -bin.zip)"
CACHE="${HOME}/.m2/wrapper/dists/${NAME}"
MVN="${CACHE}/bin/mvn"
if [[ ! -x "$MVN" ]]; then
  mkdir -p "$CACHE"
  TMP="$(mktemp -d)"
  curl -fsSL "$DIST_URL" -o "$TMP/maven.zip"
  unzip -q "$TMP/maven.zip" -d "$TMP"
  INNER="$(find "$TMP" -mindepth 1 -maxdepth 1 -type d | head -n1)"
  cp -R "$INNER"/. "$CACHE"/
  rm -rf "$TMP"
fi
exec "$MVN" -f "$ROOT/pom.xml" "$@"
