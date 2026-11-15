// SPDX-License-Identifier: GPL-3.0-or-later
import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type IssueState = 'all' | 'open' | 'unresolved' | 'resolved' | 'muted';
export type IssueSort = 'last_seen' | 'digested_event_count' | 'digest_order';
export type SortOrder = 'asc' | 'desc';

export type QueryType = 'issues' | 'events' | 'volume';

export interface VolumeSeries {
  times: number[];
  counts: number[];
  total: number;
}

export interface BugsinkQuery extends DataQuery {
  project?: number;
  issue?: string;
  eventId?: string;
  state?: IssueState;
  q?: string;
  sort?: IssueSort;
  order?: SortOrder;
}

export const DEFAULT_QUERY: Partial<BugsinkQuery> = {
  queryType: 'issues',
  state: 'unresolved',
  sort: 'last_seen',
  order: 'desc',
};

export interface BugsinkOptions extends DataSourceJsonData {
  browserUrl?: string;
  tempoDatasourceUid?: string;
  lokiDatasourceUid?: string;
  linkWindowMinutes?: number;
  logsQuery?: string;
  serviceTag?: string;
  triageEnabled?: boolean;
  hideStacktraceText?: boolean;
  attributionComments?: boolean;
}

export type TriageAction =
  'resolve' | 'reopen' | 'mute' | 'unmute' | 'resolve-latest' | 'resolve-next' | 'mute-for' | 'mute-until';

export type MuteAction = 'mute-for' | 'mute-until';

export type Period = 'minute' | 'hour' | 'day' | 'week' | 'month' | 'year';

export const PERIODS: Period[] = ['minute', 'hour', 'day', 'week', 'month', 'year'];

export interface MuteTerms {
  period: Period;
  periods: number;
  threshold?: number;
}

export interface ExploreLink {
  title: string;
  internal: {
    datasourceUid: string;
    query: Record<string, unknown>;
    timeRange: { from: string; to: string };
  };
}

export interface BugsinkSecureJsonData {
  apiToken?: string;
}

export interface Project {
  id: number;
  name: string;
}

export type IssueStatus = 'open' | 'resolved' | 'muted';

export interface IssueRow {
  id: string;
  friendlyId: string;
  type: string;
  message: string;
  transaction: string;
  project: string;
  projectId: number;
  status: IssueStatus;
  events: number;
  firstSeen: number;
  lastSeen: number;
  url?: string;
}

export interface IssueList {
  rows: IssueRow[];
  notice?: string;
}

export interface EventRow {
  id: string;
  eventId: string;
  issue: string;
  issueId: string;
  project: string;
  projectId: number;
  timestamp: number;
  ingestedAt: number;
  url?: string;
}

export interface EventList {
  rows: EventRow[];
  notices: string[];
}

export interface IssueDetail {
  id: string;
  friendlyId: string;
  projectId: number;
  project: string;
  type: string;
  message: string;
  transaction: string;
  status: IssueStatus;
  firstSeen: string;
  lastSeen: string;
  events: number;
  storedEvents: number;
  isResolved: boolean;
  isResolvedByNextRelease: boolean;
  isMuted: boolean;
  unmuteAfter: string | null;
  bugsinkUrl: string;
}

export interface TriageResult extends IssueDetail {
  warning?: string;
}

export interface IssueTag {
  key: string;
  values: Array<{ value: string; count: number }>;
  otherValues: number;
}

export interface IssueTags {
  tags: IssueTag[];
  truncated: boolean;
}

export interface HistoryEntry {
  id: number;
  kind: string;
  timestamp: string;
  user: string | null;
  comment: string;
  details: HistoryDetails;
}

export interface HistoryDetails {
  release?: string;
  nextRelease?: boolean;
  period?: Period;
  periods?: number;
  threshold?: number;
  unmuteAfter?: string;
}

export interface IssueHistory {
  history: HistoryEntry[];
  truncated: boolean;
}

export interface KeyValue {
  key: string;
  value: string;
}

export interface StackFrame {
  filename: string;
  function: string;
  module: string;
  lineno: number | null;
  colno: number | null;
  inApp: boolean;
  preContext: string[];
  contextLine: string | null;
  postContext: string[];
}

export interface EventException {
  type: string;
  value: string;
  module: string;
  mechanism: string;
  handled: boolean | null;
  frames: StackFrame[];
  framesOmitted: number;
}

export interface Breadcrumb {
  timestamp: string | null;
  type: string;
  category: string;
  level: string;
  message: string;
  data: KeyValue[];
}

export interface EventContext {
  name: string;
  fields: KeyValue[];
}

export interface EventDetail {
  id: string;
  eventId: string;
  issueId: string;
  projectId: number;
  project: string;
  timestamp: string;
  ingestedAt: string;
  platform: string;
  level: string;
  message: string;
  transaction: string;
  environment: string;
  release: string;
  traceId: string;
  tags: KeyValue[];
  contexts: EventContext[];
  exceptions: EventException[];
  breadcrumbs: Breadcrumb[];
  stacktraceMd: string | null;
  notices: string[];
  bugsinkUrl: string;
  links: ExploreLink[];
}
