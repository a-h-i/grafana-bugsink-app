// SPDX-License-Identifier: GPL-3.0-or-later
import { BugsinkQuery, DEFAULT_QUERY, IssueSort, IssueState, SortOrder } from '../../datasource/types';

export interface ListState {
  ds?: string;
  query: BugsinkQuery;
  from: string;
  to: string;
}

export const DEFAULT_FROM = 'now-7d';
export const DEFAULT_TO = 'now';

const OWN_KEYS = new Set(['ds', 'project', 'state', 'q', 'sort', 'order', 'from', 'to']);

function readProject(value: string | null): number | undefined {
  const project = Number(value);
  return Number.isInteger(project) && project > 0 ? project : undefined;
}

export function readListState(params: URLSearchParams): ListState {
  const project = readProject(params.get('project'));
  const q = params.get('q');
  return {
    ds: params.get('ds') || undefined,
    query: {
      ...DEFAULT_QUERY,
      refId: 'issues',
      ...(project && { project }),
      ...(q && { q }),
      state: (params.get('state') || DEFAULT_QUERY.state) as IssueState,
      sort: (params.get('sort') || DEFAULT_QUERY.sort) as IssueSort,
      order: (params.get('order') || DEFAULT_QUERY.order) as SortOrder,
    },
    from: params.get('from') || DEFAULT_FROM,
    to: params.get('to') || DEFAULT_TO,
  };
}

export function writeListState({ ds, query, from, to }: ListState): URLSearchParams {
  const params = new URLSearchParams();
  if (ds) {
    params.set('ds', ds);
  }
  if (query.project) {
    params.set('project', String(query.project));
  }
  params.set('state', query.state ?? DEFAULT_QUERY.state!);
  if (query.q) {
    params.set('q', query.q);
  }
  params.set('sort', query.sort ?? DEFAULT_QUERY.sort!);
  params.set('order', query.order ?? DEFAULT_QUERY.order!);
  params.set('from', from);
  params.set('to', to);
  return params;
}

function isOwn(segment: string): boolean {
  try {
    return OWN_KEYS.has(decodeURIComponent(segment.split('=')[0].replace(/\+/g, ' ')));
  } catch {
    return false;
  }
}

/** Returns the search string for a state, keeping the parameters of `search` the page does not own verbatim. */
export function listSearch(state: ListState, search: string): string {
  const foreign = search
    .replace(/^\?/, '')
    .split('&')
    .filter((segment) => segment && !isOwn(segment));
  return `?${[...foreign, writeListState(state).toString()].join('&')}`;
}
