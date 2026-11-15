#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Fail when a tracked source file lacks the SPDX header in its first five lines.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

missing=0
while IFS= read -r -d '' file; do
  if ! head -n 5 "$file" | grep -q 'SPDX-License-Identifier: GPL-3.0-or-later'; then
    echo "missing SPDX header: $file"
    missing=1
  fi
done < <(git ls-files -z -- '*.go' '*.ts' '*.tsx' '*.js' '*.mjs' '*.sh' '*.py' '*.yaml' '*.yml' \
  ':!:.config/*' ':!:pkg/bugsink/internal/gen/*' ':!:bugsink-api/*' ':!:node_modules/*')

exit "$missing"
