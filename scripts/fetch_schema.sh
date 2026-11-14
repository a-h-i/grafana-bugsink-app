#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Download Bugsink's OpenAPI schema (YAML, no token needed) into bugsink-api/openapi.yaml, unmodified.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
url="${BUGSINK_URL:-http://localhost:8800}/api/canonical/0/schema/"
out="$root/bugsink-api/openapi.yaml"

tmp="$(mktemp "$root/bugsink-api/.openapi.yaml.XXXXXX")"
trap 'rm -f "$tmp"' EXIT

curl --fail --silent --show-error --max-time 30 --output "$tmp" "$url"

if ! grep -Eq '^openapi: ' "$tmp"; then
  echo "fetch_schema: $url did not return an OpenAPI YAML document" >&2
  exit 1
fi

chmod 644 "$tmp"
mv "$tmp" "$out"
echo "wrote $out"
