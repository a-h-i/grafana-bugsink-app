// SPDX-License-Identifier: GPL-3.0-or-later
export const APP_ROOT = '/a/ahmedismail-bugsink-app';

export function issuePath(id: string, search: string): string {
  return `${APP_ROOT}/issues/${encodeURIComponent(id)}${search}`;
}

export function listPath(search: string): string {
  return `${APP_ROOT}${search}`;
}
