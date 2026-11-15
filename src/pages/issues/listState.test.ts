// SPDX-License-Identifier: GPL-3.0-or-later
import { DEFAULT_QUERY } from '../../datasource/types';
import { listSearch, readListState, writeListState } from './listState';

describe('readListState', () => {
  it('gives the defaults for empty params', () => {
    const state = readListState(new URLSearchParams());

    expect(state.ds).toBeUndefined();
    expect(state.query).toEqual({ ...DEFAULT_QUERY, refId: 'issues' });
    expect(state.from).toBe('now-7d');
    expect(state.to).toBe('now');
  });

  it('round-trips a full state', () => {
    const search = 'ds=eu&project=2&state=open&q=a+b&sort=digested_event_count&order=asc&from=now-24h&to=now';

    const state = readListState(new URLSearchParams(search));

    expect(state.ds).toBe('eu');
    expect(state.query).toMatchObject({
      project: 2,
      state: 'open',
      q: 'a b',
      sort: 'digested_event_count',
      order: 'asc',
    });
    expect(state.from).toBe('now-24h');
    expect(Object.fromEntries(writeListState(state))).toEqual({
      ds: 'eu',
      project: '2',
      state: 'open',
      q: 'a b',
      sort: 'digested_event_count',
      order: 'asc',
      from: 'now-24h',
      to: 'now',
    });
  });

  it.each(['abc', '0', '-1'])('ignores project=%s', (value) => {
    expect(readListState(new URLSearchParams({ project: value })).query.project).toBeUndefined();
  });

  it('keeps an unknown state', () => {
    expect(readListState(new URLSearchParams('state=bogus')).query.state).toBe('bogus');
  });
});

describe('writeListState', () => {
  it('leaves out project and q when unset', () => {
    const params = writeListState(readListState(new URLSearchParams('ds=bugsink')));

    expect(params.toString()).toBe('ds=bugsink&state=unresolved&sort=last_seen&order=desc&from=now-7d&to=now');
  });
});

describe('listSearch', () => {
  const own = 'ds=eu&state=open&sort=last_seen&order=desc&from=now-7d&to=now';

  it('keeps the parameters it does not own verbatim, a bare key included', () => {
    const search = '?orgId=1&kiosk&var-x=a%20b&ds=eu&state=open';

    expect(listSearch(readListState(new URLSearchParams(search)), search)).toBe(`?orgId=1&kiosk&var-x=a%20b&${own}`);
  });

  it('replaces its own parameters with the state', () => {
    const search = '?state=muted&orgId=2&project=abc&q=old';
    const state = readListState(new URLSearchParams(own));

    expect(listSearch(state, search)).toBe(`?orgId=2&${own}`);
  });

  it('gives the same string when applied to its own result', () => {
    const search = '?kiosk&orgId=1&q=a+b&ds=eu';
    const once = listSearch(readListState(new URLSearchParams(search)), search);

    expect(listSearch(readListState(new URLSearchParams(once)), once)).toBe(once);
  });
});
