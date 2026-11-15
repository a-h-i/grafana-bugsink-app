// SPDX-License-Identifier: GPL-3.0-or-later
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';

const file = (name: string) => `playwright/.${name}.json`;

export function writeReported(name: string, value: unknown) {
  mkdirSync('playwright', { recursive: true });
  writeFileSync(file(name), JSON.stringify(value));
}

export function readReported<T>(name: string): T {
  return JSON.parse(readFileSync(file(name), 'utf8'));
}
