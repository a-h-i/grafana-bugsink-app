// SPDX-License-Identifier: GPL-3.0-or-later
// Public development-only values from docker/bugsink/seed.py.
export const BUGSINK_URL = 'http://bugsink:8000';
export const GLOBAL_READ = 'a'.repeat(40);
export const PROJECT_READ = 'c'.repeat(40);
export const NO_CAPS = 'd'.repeat(40);
export const SAMPLE_APP_URL = process.env.SAMPLE_APP_URL || 'http://localhost:8810';
export const BUGSINK_BROWSER_URL = 'http://localhost:8800';
export const EVENT_MARKER = 'm2-e2e';
export const TRACE_MARKER = 'm4-trace';
