// SPDX-License-Identifier: GPL-3.0-or-later
import {
  createDataFrame,
  DataLink,
  DataSourceInstanceSettings,
  FieldType,
  ScopedVars,
  TimeRange,
  dateTime,
} from '@grafana/data';
import { DataSourceWithBackend, setTemplateSrv } from '@grafana/runtime';
import { lastValueFrom, of } from 'rxjs';
import { BugsinkDataSource } from './datasource';
import { BugsinkOptions, DEFAULT_QUERY } from './types';

const settings = { uid: 'bugsink', name: 'Bugsink', jsonData: {} } as DataSourceInstanceSettings<BugsinkOptions>;

function createDataSource() {
  setTemplateSrv({
    replace: (target?: string, scopedVars?: ScopedVars) =>
      (target ?? '').replace('$env', String(scopedVars?.env?.value ?? '')),
    getVariables: () => [],
    containsTemplate: () => false,
    updateTimeRange: () => {},
  });
  return new BugsinkDataSource(settings);
}

describe('BugsinkDataSource', () => {
  it('starts an empty query from the default query', () => {
    expect(createDataSource().getDefaultQuery()).toEqual(DEFAULT_QUERY);
  });

  it('interpolates variables in the search text only', () => {
    const query = {
      refId: 'A',
      project: 2,
      state: 'open',
      q: 'environment:$env',
      sort: 'last_seen',
      order: 'asc',
    } as const;

    const result = createDataSource().applyTemplateVariables(query, { env: { text: 'prod', value: 'production' } });

    expect(result).toEqual({ ...query, q: 'environment:production' });
  });

  it('interpolates $env in q, issue and eventId', () => {
    const query = { refId: 'A', queryType: 'events', q: 'trace:$env', issue: '$env', eventId: '$env' };

    const result = createDataSource().applyTemplateVariables(query, { env: { text: 't', value: 'abc' } });

    expect(result).toEqual({ ...query, q: 'trace:abc', issue: 'abc', eventId: 'abc' });
  });

  it.each([
    ['getIssue', 'issues/SAMPLE-PYTHON-2'],
    ['getIssueTags', 'issues/SAMPLE-PYTHON-2/tags'],
    ['getIssueHistory', 'issues/SAMPLE-PYTHON-2/history'],
  ] as const)('%s reads %s', async (method, path) => {
    const ds = createDataSource();
    const getResource = jest.spyOn(ds, 'getResource').mockResolvedValue({ ok: true });

    await expect(ds[method]('SAMPLE-PYTHON-2')).resolves.toEqual({ ok: true });
    expect(getResource).toHaveBeenCalledWith(path, undefined, { showErrorAlert: false });
  });

  it('reads an event with its id encoded', async () => {
    const ds = createDataSource();
    const getResource = jest.spyOn(ds, 'getResource').mockResolvedValue({ id: 'a/b' });

    await expect(ds.getEvent('a/b')).resolves.toEqual({ id: 'a/b' });
    expect(getResource).toHaveBeenCalledWith('events/a%2Fb', undefined, { showErrorAlert: false });
  });

  it('encodes the issue id in the resource path', async () => {
    const ds = createDataSource();
    const getResource = jest.spyOn(ds, 'getResource').mockResolvedValue({});

    await ds.getIssue('a/b');

    expect(getResource).toHaveBeenCalledWith('issues/a%2Fb', undefined, { showErrorAlert: false });
  });

  describe('query', () => {
    afterEach(() => jest.restoreAllMocks());

    it('gives link time ranges to the frames the backend answers', async () => {
      const frame = createDataFrame({
        fields: [
          {
            name: 'trace_id',
            type: FieldType.string,
            values: ['abc'],
            config: {
              links: [
                {
                  title: 'Open trace',
                  url: '',
                  internal: {
                    datasourceUid: 'tempo',
                    query: {},
                    timeRange: { from: '2026-11-15T09:45:00Z', to: '2026-11-15T10:20:00Z' },
                  },
                },
              ] as unknown as DataLink[],
            },
          },
        ],
      });
      jest.spyOn(DataSourceWithBackend.prototype, 'query').mockReturnValue(of({ data: [frame] }));

      const response = await lastValueFrom(createDataSource().query({ targets: [] } as never));

      const link = response.data[0].fields[0].config.links[0];
      expect(link.internal.range.raw.from.valueOf()).toBe(Date.parse('2026-11-15T09:45:00Z'));
    });
  });

  describe('listEvents', () => {
    function eventsFrame(notices: string[] = []) {
      const col = (name: string, type: FieldType, value: string | number) => ({ name, type, values: [value] });
      return createDataFrame({
        fields: [
          col('timestamp', FieldType.time, 1000),
          col('event_id', FieldType.string, 'sdk-1'),
          col('issue', FieldType.string, 'X-1'),
          col('project', FieldType.string, 'p'),
          col('ingested_at', FieldType.time, 1500),
          col('id', FieldType.string, 'e1'),
          col('issue_id', FieldType.string, 'i1'),
          col('project_id', FieldType.number, 1),
        ],
        meta: { notices: notices.map((text) => ({ severity: 'warning' as const, text })) },
      });
    }

    it('sends one events target', async () => {
      const ds = createDataSource();
      const query = jest.spyOn(ds, 'query').mockReturnValue(of({ data: [eventsFrame()] }));

      await ds.listEvents({ refId: 'x', issue: 'X-1' });

      expect(query.mock.calls[0][0].targets).toEqual([
        { queryType: 'events', issue: 'X-1', refId: 'events', datasource: ds.getRef() },
      ]);
    });

    it('resolves the rows and every notice in order', async () => {
      const ds = createDataSource();
      jest.spyOn(ds, 'query').mockReturnValue(of({ data: [eventsFrame(['first', 'second'])] }));

      const list = await ds.listEvents({ refId: 'x', issue: 'X-1' });

      expect(list.notices).toEqual(['first', 'second']);
      expect(list.rows).toEqual([expect.objectContaining({ id: 'e1', eventId: 'sdk-1', issueId: 'i1' })]);
    });

    it('rejects with the message of a refused query', async () => {
      const ds = createDataSource();
      const message = 'An events query needs an issue, an event ID or trace:<id> in Search.';
      jest.spyOn(ds, 'query').mockReturnValue(of({ data: [], error: { message } }));

      await expect(ds.listEvents({ refId: 'x' })).rejects.toThrow(message);
    });

    it('rejects with a generic message when the error has none', async () => {
      const ds = createDataSource();
      jest.spyOn(ds, 'query').mockReturnValue(of({ data: [], error: { status: 502 } }));

      await expect(ds.listEvents({ refId: 'x', issue: 'X-1' })).rejects.toThrow('The events could not be loaded.');
    });
  });

  it('reads the project list from the projects resource', async () => {
    const projects = [{ id: 2, name: 'sample-js' }];
    const ds = createDataSource();
    const getResource = jest.spyOn(ds, 'getResource').mockResolvedValue(projects);

    await expect(ds.getProjects()).resolves.toEqual(projects);
    expect(getResource).toHaveBeenCalledWith('projects', undefined, { showErrorAlert: false });
  });

  describe('listIssues', () => {
    const now = dateTime('2026-11-15T12:00:00Z');
    const range: TimeRange = { from: dateTime('2026-11-08T12:00:00Z'), to: now, raw: { from: 'now-7d', to: 'now' } };
    const NOTICE = 'Only the first 5000 issues were read. Narrow the search or the time range.';

    function issuesFrame(notices?: Array<{ severity: 'warning'; text: string }>) {
      const names = ['friendly_id', 'calculated_type', 'calculated_value', 'transaction', 'project', 'status'];
      const fields = [
        ...names.map((name, i) => ({
          name,
          type: FieldType.string,
          values: [['S-1', 'T', 'm', '/x', 'p', 'open'][i]],
        })),
        { name: 'digested_event_count', type: FieldType.number, values: [4] },
        { name: 'first_seen', type: FieldType.time, values: [1000] },
        { name: 'last_seen', type: FieldType.time, values: [2000] },
        { name: 'project_id', type: FieldType.number, values: [1] },
        { name: 'id', type: FieldType.string, values: ['abc'] },
      ];
      return createDataFrame({ fields, meta: notices && { notices } });
    }

    it('sends one issues target with the defaults and the given range', async () => {
      const ds = createDataSource();
      const query = jest.spyOn(ds, 'query').mockReturnValue(of({ data: [issuesFrame()] }));

      await ds.listIssues({ refId: 'x', project: 1 }, range);

      const request = query.mock.calls[0][0];
      expect(request.targets).toEqual([
        {
          queryType: 'issues',
          state: 'unresolved',
          sort: 'last_seen',
          order: 'desc',
          project: 1,
          refId: 'issues',
          datasource: ds.getRef(),
        },
      ]);
      expect(request.range).toBe(range);
    });

    it('resolves the rows and the frame notice', async () => {
      const ds = createDataSource();
      jest.spyOn(ds, 'query').mockReturnValue(of({ data: [issuesFrame([{ severity: 'warning', text: NOTICE }])] }));

      const list = await ds.listIssues({ refId: 'x' }, range);

      expect(list.notice).toBe(NOTICE);
      expect(list.rows).toEqual([expect.objectContaining({ id: 'abc', friendlyId: 'S-1', events: 4 })]);
    });

    it('rejects with the message of a refused query', async () => {
      const ds = createDataSource();
      const message = 'Bugsink refused the request: No Project matches the given query.';
      jest.spyOn(ds, 'query').mockReturnValue(of({ data: [], error: { message } }));

      await expect(ds.listIssues({ refId: 'x', project: 99 }, range)).rejects.toThrow(message);
    });

    it('rejects with a generic message when the error has none', async () => {
      const ds = createDataSource();
      jest.spyOn(ds, 'query').mockReturnValue(of({ data: [], error: { status: 502 } }));

      await expect(ds.listIssues({ refId: 'x' }, range)).rejects.toThrow('The issues could not be loaded.');
    });
  });
});
