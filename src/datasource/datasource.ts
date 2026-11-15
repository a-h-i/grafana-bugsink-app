// SPDX-License-Identifier: GPL-3.0-or-later
import {
  CoreApp,
  DataFrame,
  DataSourceInstanceSettings,
  getDefaultTimeRange,
  ScopedVars,
  TimeRange,
} from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';
import { lastValueFrom } from 'rxjs';
import { eventRows } from './eventRows';
import { issueRows } from './issueRows';
import {
  BugsinkOptions,
  BugsinkQuery,
  DEFAULT_QUERY,
  EventDetail,
  EventList,
  IssueDetail,
  IssueHistory,
  IssueList,
  IssueTags,
  Project,
} from './types';

const APP_ID = 'ahmedismail-bugsink-app';
const ISSUES_FAILED = 'The issues could not be loaded.';
const EVENTS_FAILED = 'The events could not be loaded.';
const QUIET = { showErrorAlert: false };

function failureMessage(error: { message?: string; data?: { message?: string } }, fallback: string): string {
  return error.message || error.data?.message || fallback;
}

export class BugsinkDataSource extends DataSourceWithBackend<BugsinkQuery, BugsinkOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<BugsinkOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(): Partial<BugsinkQuery> {
    return DEFAULT_QUERY;
  }

  applyTemplateVariables(query: BugsinkQuery, scopedVars: ScopedVars): BugsinkQuery {
    const replace = (text?: string) => getTemplateSrv().replace(text, scopedVars);
    return {
      ...query,
      q: replace(query.q),
      ...(query.issue !== undefined && { issue: replace(query.issue) }),
      ...(query.eventId !== undefined && { eventId: replace(query.eventId) }),
    };
  }

  getProjects(): Promise<Project[]> {
    return this.getResource('projects', undefined, QUIET);
  }

  getIssue(id: string): Promise<IssueDetail> {
    return this.getResource(`issues/${encodeURIComponent(id)}`, undefined, QUIET);
  }

  getEvent(id: string): Promise<EventDetail> {
    return this.getResource(`events/${encodeURIComponent(id)}`, undefined, QUIET);
  }

  getIssueTags(id: string): Promise<IssueTags> {
    return this.getResource(`issues/${encodeURIComponent(id)}/tags`, undefined, QUIET);
  }

  getIssueHistory(id: string): Promise<IssueHistory> {
    return this.getResource(`issues/${encodeURIComponent(id)}/history`, undefined, QUIET);
  }

  async listIssues(query: BugsinkQuery, range: TimeRange): Promise<IssueList> {
    const frames = await this.runTarget({ ...DEFAULT_QUERY, ...query, refId: 'issues' }, range, ISSUES_FAILED);
    const frame = frames[0];
    return { rows: frame ? issueRows(frame) : [], notice: frame?.meta?.notices?.[0]?.text };
  }

  async listEvents(query: Omit<BugsinkQuery, 'refId'> & { refId?: string }): Promise<EventList> {
    const frames = await this.runTarget(
      { ...query, queryType: 'events', refId: 'events' },
      getDefaultTimeRange(),
      EVENTS_FAILED
    );
    const frame = frames[0];
    return {
      rows: frame ? eventRows(frame) : [],
      notices: frame?.meta?.notices?.map((notice) => notice.text) ?? [],
    };
  }

  private async runTarget(target: BugsinkQuery, range: TimeRange, failed: string): Promise<DataFrame[]> {
    const response = await lastValueFrom(
      this.query({
        targets: [{ ...target, datasource: this.getRef() }],
        range,
        rangeRaw: range.raw,
        app: APP_ID as CoreApp,
        requestId: `${target.refId}-${Math.random().toString(36).slice(2)}`,
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
      throw new Error(failureMessage(error, failed));
    }
    return response.data;
  }
}
