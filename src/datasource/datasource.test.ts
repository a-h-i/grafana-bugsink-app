// SPDX-License-Identifier: GPL-3.0-or-later
import { DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { setTemplateSrv } from '@grafana/runtime';
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
});
