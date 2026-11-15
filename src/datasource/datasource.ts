// SPDX-License-Identifier: GPL-3.0-or-later
import {
  CoreApp,
  DataFrame,
  DataQueryRequest,
  DataQueryResponse,
  DataSourceInstanceSettings,
  dateTime,
  getDefaultTimeRange,
  ScopedVars,
  TimeRange,
} from '@grafana/data';
import { config, DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';
import { map, Observable } from 'rxjs';
import { eventRows } from './eventRows';
import { issueRows } from './issueRows';
import { abortError, limiter } from './limiter';
import { withLinkRanges } from './links';
import { muteTermsError, SHAPE_MESSAGE } from './muteTerms';
import { volumeSeries } from './volume';
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
  VolumeSeries,
} from './types';

const APP_ID = 'ahmedismail-bugsink-app';
const ISSUES_FAILED = 'The issues could not be loaded.';
const EVENTS_FAILED = 'The events could not be loaded.';
const QUIET = { showErrorAlert: false };
const VOLUME_FAILED = 'The event volume could not be loaded.';
const HOUR_MS = 3600000;
const VOLUME_HOURS = 24;
export const SPARKLINE_CONCURRENCY = 4;
const ISSUE_ID = /^[A-Za-z0-9_-]{1,100}$/;

export function isIssueId(id: string): boolean {
  return ISSUE_ID.test(id);
}

function assertIssueId(id: string): void {
  if (!isIssueId(id)) {
    throw new Error(`"${id}" is not an issue id.`);
  }
}

function issuePath(id: string): string {
  assertIssueId(id);
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
  private readonly limitVolume = limiter(SPARKLINE_CONCURRENCY);

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

  async listIssues(query: BugsinkQuery, range: TimeRange, signal?: AbortSignal): Promise<IssueList> {
    const frames = await this.runTarget({ ...DEFAULT_QUERY, ...query, refId: 'issues' }, range, ISSUES_FAILED, signal);
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

  async issueVolume(id: string, signal: AbortSignal): Promise<VolumeSeries> {
    assertIssueId(id);
    return this.limitVolume(async (taskSignal) => {
      const now = Date.now();
      const from = Math.floor(now / HOUR_MS) * HOUR_MS - (VOLUME_HOURS - 1) * HOUR_MS;
      const range: TimeRange = { from: dateTime(from), to: dateTime(now), raw: { from: 'now-24h', to: 'now' } };
      const frames = await this.runTarget(
        { queryType: 'volume', issue: id, refId: 'volume' },
        range,
        VOLUME_FAILED,
        taskSignal,
        { interval: '1h', intervalMs: HOUR_MS, maxDataPoints: VOLUME_HOURS }
      );
      return frames[0] ? volumeSeries(frames[0]) : { times: [], counts: [], total: 0 };
    }, signal);
  }

  private runTarget(
    target: BugsinkQuery,
    range: TimeRange,
    failed: string,
    signal?: AbortSignal,
    shape: { interval: string; intervalMs: number; maxDataPoints?: number } = { interval: '1m', intervalMs: 60000 }
  ): Promise<DataFrame[]> {
    return new Promise((resolve, reject) => {
      let last: DataQueryResponse | undefined;
      const subscription = this.query({
        targets: [{ ...target, datasource: this.getRef() }],
        range,
        rangeRaw: range.raw,
        app: APP_ID as CoreApp,
        requestId: `${target.refId}-${Math.random().toString(36).slice(2)}`,
        interval: shape.interval,
        intervalMs: shape.intervalMs,
        ...(shape.maxDataPoints !== undefined && { maxDataPoints: shape.maxDataPoints }),
        scopedVars: {},
        timezone: 'browser',
        startTime: Date.now(),
      }).subscribe({
        next: (response) => {
          last = response;
        },
        error: reject,
        complete: () => {
          // eslint-disable-next-line @typescript-eslint/no-deprecated -- a failed fetch sets only error
          const error = last?.error;
          if (error) {
            reject(new Error(failureMessage(error, failed)));
          } else {
            resolve(last?.data ?? []);
          }
        },
      });
      const onAbort = () => {
        subscription.unsubscribe();
        reject(abortError());
      };
      if (signal?.aborted) {
        onAbort();
      } else {
        signal?.addEventListener('abort', onAbort, { once: true });
        subscription.add(() => signal?.removeEventListener('abort', onAbort));
      }
    });
  }
}
