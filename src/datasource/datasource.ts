// SPDX-License-Identifier: GPL-3.0-or-later
import {
  CoreApp,
  DataFrame,
  DataQueryRequest,
  DataQueryResponse,
  DataSourceInstanceSettings,
  getDefaultTimeRange,
  ScopedVars,
  TimeRange,
} from '@grafana/data';
import { config, DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';
import { lastValueFrom, map, Observable } from 'rxjs';
import { eventRows } from './eventRows';
import { issueRows } from './issueRows';
import { withLinkRanges } from './links';
import { muteTermsError, SHAPE_MESSAGE } from './muteTerms';
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
  MuteAction,
  MuteTerms,
  Project,
  TriageAction,
  TriageResult,
} from './types';

const APP_ID = 'ahmedismail-bugsink-app';
const ISSUES_FAILED = 'The issues could not be loaded.';
const EVENTS_FAILED = 'The events could not be loaded.';
const QUIET = { showErrorAlert: false };
const ISSUE_ID = /^[A-Za-z0-9_-]{1,100}$/;

export function isIssueId(id: string): boolean {
  return ISSUE_ID.test(id);
}

function issuePath(id: string): string {
  if (!isIssueId(id)) {
    throw new Error(`"${id}" is not an issue id.`);
  }
  return `issues/${id}`;
}

// muteBody is the request body of a mute action, holding only the validated terms.
function muteBody(action: MuteAction, terms: MuteTerms | undefined): MuteTerms {
  if (!terms) {
    throw new Error(SHAPE_MESSAGE[action]);
  }
  const refusal = muteTermsError(action, terms);
  if (refusal) {
    throw new Error(refusal);
  }
  const { period, periods, threshold } = terms;
  return action === 'mute-until' ? { period, periods, threshold } : { period, periods };
}

function failureMessage(error: { message?: string; data?: { message?: string } }, fallback: string): string {
  return error.message || error.data?.message || fallback;
}

export class BugsinkDataSource extends DataSourceWithBackend<BugsinkQuery, BugsinkOptions> {
  readonly triageEnabled: boolean;

  constructor(instanceSettings: DataSourceInstanceSettings<BugsinkOptions>) {
    super(instanceSettings);
    this.triageEnabled = instanceSettings.jsonData?.triageEnabled === true;
  }

  canTriage(): boolean {
    const role = config.bootData.user.orgRole;
    return this.triageEnabled && (role === 'Editor' || role === 'Admin');
  }

  getDefaultQuery(): Partial<BugsinkQuery> {
    return DEFAULT_QUERY;
  }

  query(request: DataQueryRequest<BugsinkQuery>): Observable<DataQueryResponse> {
    return super.query(request).pipe(map((response) => ({ ...response, data: withLinkRanges(response.data) })));
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

  async getIssue(id: string): Promise<IssueDetail> {
    return this.getResource(issuePath(id), undefined, QUIET);
  }

  getEvent(id: string): Promise<EventDetail> {
    return this.getResource(`events/${encodeURIComponent(id)}`, undefined, QUIET);
  }

  async getIssueTags(id: string): Promise<IssueTags> {
    return this.getResource(`${issuePath(id)}/tags`, undefined, QUIET);
  }

  async getIssueHistory(id: string): Promise<IssueHistory> {
    return this.getResource(`${issuePath(id)}/history`, undefined, QUIET);
  }

  async triage(id: string, action: TriageAction, terms?: MuteTerms): Promise<TriageResult> {
    const path = `${issuePath(id)}/${action}`;
    const body = action === 'mute-for' || action === 'mute-until' ? muteBody(action, terms) : undefined;
    return this.postResource(path, body, {
      ...QUIET,
      headers: { 'X-Bugsink-Triage': '1' },
    });
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
