// SPDX-License-Identifier: GPL-3.0-or-later
import { CoreApp, DataSourceInstanceSettings, ScopedVars, TimeRange } from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';
import { lastValueFrom } from 'rxjs';
import { issueRows } from './issueRows';
import { BugsinkOptions, BugsinkQuery, DEFAULT_QUERY, IssueList, Project } from './types';

const APP_ID = 'ahmedismail-bugsink-app';
const LOAD_FAILED = 'The issues could not be loaded.';

function failureMessage(error: { message?: string; data?: { message?: string } }): string {
  return error.message || error.data?.message || LOAD_FAILED;
}

export class BugsinkDataSource extends DataSourceWithBackend<BugsinkQuery, BugsinkOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<BugsinkOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(): Partial<BugsinkQuery> {
    return DEFAULT_QUERY;
  }

  applyTemplateVariables(query: BugsinkQuery, scopedVars: ScopedVars): BugsinkQuery {
    return { ...query, q: getTemplateSrv().replace(query.q, scopedVars) };
  }

  getProjects(): Promise<Project[]> {
    return this.getResource('projects');
  }

  async listIssues(query: BugsinkQuery, range: TimeRange): Promise<IssueList> {
    const response = await lastValueFrom(
      this.query({
        targets: [{ ...DEFAULT_QUERY, ...query, refId: 'issues', datasource: this.getRef() }],
        range,
        rangeRaw: range.raw,
        app: APP_ID as CoreApp,
        requestId: `issues-${Math.random().toString(36).slice(2)}`,
        interval: '1m',
        intervalMs: 60000,
        scopedVars: {},
        timezone: 'browser',
        startTime: Date.now(),
      })
    );
    // eslint-disable-next-line @typescript-eslint/no-deprecated -- a failed fetch sets only error
    const error = response.error;
    if (error) {
      throw new Error(failureMessage(error));
    }
    const frame = response.data[0];
    return { rows: frame ? issueRows(frame) : [], notice: frame?.meta?.notices?.[0]?.text };
  }
}
