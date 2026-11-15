// SPDX-License-Identifier: GPL-3.0-or-later
import { createDataFrame, DataSourceInstanceSettings, FieldType, ScopedVars, TimeRange, dateTime } from '@grafana/data';
import { setTemplateSrv } from '@grafana/runtime';
import { of } from 'rxjs';
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

  it('reads the project list from the projects resource', async () => {
    const projects = [{ id: 2, name: 'sample-js' }];
    const ds = createDataSource();
    const getResource = jest.spyOn(ds, 'getResource').mockResolvedValue(projects);

    await expect(ds.getProjects()).resolves.toEqual(projects);
    expect(getResource).toHaveBeenCalledWith('projects');
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
