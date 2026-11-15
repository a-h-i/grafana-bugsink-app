// SPDX-License-Identifier: GPL-3.0-or-later
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';

export const TRACE_FILE = 'playwright/.trace.json';

export type ReportedTrace = { traceId: string; eventId: string };

export function writeTrace(trace: ReportedTrace) {
  mkdirSync(dirname(TRACE_FILE), { recursive: true });
  writeFileSync(TRACE_FILE, JSON.stringify(trace));
}

export function readTrace(): ReportedTrace {
  return JSON.parse(readFileSync(TRACE_FILE, 'utf8'));
}
