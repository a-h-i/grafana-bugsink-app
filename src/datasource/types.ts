// SPDX-License-Identifier: GPL-3.0-or-later
import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type IssueState = 'all' | 'open' | 'unresolved' | 'resolved' | 'muted';
export type IssueSort = 'last_seen' | 'digested_event_count' | 'digest_order';
export type SortOrder = 'asc' | 'desc';

export type QueryType = 'issues' | 'events';

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
}

export interface IssueHistory {
  history: HistoryEntry[];
  truncated: boolean;
}
