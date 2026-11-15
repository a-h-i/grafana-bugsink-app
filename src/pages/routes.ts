// SPDX-License-Identifier: GPL-3.0-or-later
export const APP_ROOT = '/a/ahmedismail-bugsink-app';

export function issuePath(id: string, search: string): string {
  return `${APP_ROOT}/issues/${encodeURIComponent(id)}${search}`;
}

export function listPath(search: string): string {
  return `${APP_ROOT}${search}`;
}

export function withEvent(search: string, eventId?: string): string {
  const added = eventId === undefined ? undefined : `event=${encodeURIComponent(eventId)}`;
  const segments: string[] = [];
  let placed = false;
  for (const segment of search.replace(/^\?/, '').split('&')) {
    if (segment === '') {
      continue;
    }
    if (segment.split('=')[0] !== 'event') {
      segments.push(segment);
    } else if (added !== undefined && !placed) {
      segments.push(added);
      placed = true;
    }
  }
  if (added !== undefined && !placed) {
    segments.push(added);
  }
  return segments.length === 0 ? '' : `?${segments.join('&')}`;
}
