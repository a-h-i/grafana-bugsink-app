#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Fail when a production npm or Go dependency is under a licence outside the allowed list.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

allowed="MIT,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,0BSD,CC0-1.0,Unlicense,MPL-2.0,BlueOak-1.0.0,Zlib,Python-2.0,LGPL-3.0-or-later,GPL-3.0-or-later,AGPL-3.0-only,AGPL-3.0-or-later"

echo "npm production dependencies"
# get-document@1.0.0 declares no licence in its package; its repository is MIT.
npx --no-install license-checker-rseidelsohn --production --excludePackages "get-document@1.0.0" --json \
  | node scripts/check_npm_licenses.mjs "$allowed"

echo "Go dependencies"
# go-licenses reports this module's own licence, so it is ignored.
go run github.com/google/go-licenses@v1.6.0 check ./pkg/... \
  --allowed_licenses="$allowed" \
  --ignore github.com/a-h-i/grafana-bugsink-app 2>&1 | { grep -v -e 'contains non-Go code' -e '\.s$' || true; }
