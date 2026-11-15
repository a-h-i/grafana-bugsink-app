// SPDX-License-Identifier: GPL-3.0-or-later
// Reads license-checker JSON on stdin and fails when a package's SPDX expression is not satisfied by the allowed list.
import { readFileSync } from 'node:fs';
import parse from 'spdx-expression-parse';
import satisfies from 'spdx-satisfies';

const allowed = process.argv[2].split(',');
const root = JSON.parse(readFileSync('package.json', 'utf8'));

// These packages ship the Unlicense text without a licence field.
const clarified = {
  'fast-shallow-equal@1.0.0': 'Unlicense',
  'react-universal-interface@0.6.2': 'Unlicense',
};

const packages = JSON.parse(readFileSync(0, 'utf8'));
let failed = false;
for (const [id, info] of Object.entries(packages)) {
  if (id === `${root.name}@${root.version}`) {
    continue;
  }
  const licence = clarified[id] ?? info.licenses;
  let ok = false;
  try {
    parse(licence);
    ok = satisfies(licence, allowed);
  } catch {
    ok = false;
  }
  if (!ok) {
    console.log(`licence not allowed: ${id}: ${licence}`);
    failed = true;
  }
}
process.exit(failed ? 1 : 0);
