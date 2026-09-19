#!/bin/sh
# Thin wrapper so workflow steps can call scripts/sdk-release.sh.
exec python3 "$(dirname "$0")/sdk_release.py" "$@"
