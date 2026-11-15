// SPDX-License-Identifier: GPL-3.0-or-later
import React, { useEffect, useState } from 'react';
import { config, PluginPage } from '@grafana/runtime';
import { Alert, Button, LinkButton, LoadingPlaceholder, RefreshPicker } from '@grafana/ui';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { BugsinkDataSource } from '../datasource/datasource';
import { EventTable } from './issue/EventTable';
import { IssueHistory } from './issue/IssueHistory';
import { IssueSummary } from './issue/IssueSummary';
import { IssueTags } from './issue/IssueTags';
import { NoDataSource } from './NoDataSource';
import { listPath } from './routes';
import { useDataSource } from './useDataSource';

type Section<T> = { source: BugsinkDataSource; issueId: string; refresh: number } & ({ value: T } | { error: string });

function messageOf(error: { message?: string; data?: { message?: string } }, fallback: string): string {
  return error?.message || error?.data?.message || fallback;
}

function useSection<T>(
  source: BugsinkDataSource | undefined,
  issueId: string,
  refresh: number,
  read: (source: BugsinkDataSource) => Promise<T>,
  fallback: string
): { pending: boolean; value?: T; error?: string } {
  const [section, setSection] = useState<Section<T>>();

  useEffect(() => {
    if (!source) {
      return;
    }
    let active = true;
    read(source)
      .then((value): Section<T> => ({ source, issueId, refresh, value }))
      .catch((error): Section<T> => ({ source, issueId, refresh, error: messageOf(error, fallback) }))
      .then((result) => active && setSection(result));
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- the read is fixed per section
  }, [source, issueId, refresh]);

  const current =
    section && section.source === source && section.issueId === issueId && section.refresh === refresh
      ? section
      : undefined;
  if (!current) {
    return { pending: source !== undefined };
  }
  return 'error' in current ? { pending: false, error: current.error } : { pending: false, value: current.value };
}

function SectionState({
  loading,
  state,
}: {
  loading: string;
  state: { pending: boolean; error?: string };
}): React.JSX.Element | null {
  if (state.pending) {
    return <LoadingPlaceholder text={loading} />;
  }
  return state.error ? <Alert severity="error" title={state.error} /> : null;
}

function PageSection({
  title,
  loading,
  state,
  children,
}: {
  title: string;
  loading: string;
  state: { pending: boolean; error?: string };
  children: React.ReactNode;
}): React.JSX.Element {
  return (
    <section>
      <h2>{title}</h2>
      <SectionState loading={loading} state={state} />
      {children}
    </section>
  );
}

export function IssuePage(): React.JSX.Element {
  const { issueId = '' } = useParams();
  const { search } = useLocation();
  const navigate = useNavigate();
  const requested = new URLSearchParams(search).get('ds') || undefined;
  const [refresh, setRefresh] = useState(0);
  const choice = useDataSource(requested, refresh);
  const { source } = choice;
  const timeZone = config.bootData.user.timezone;
  const summary = useSection(source, issueId, refresh, (s) => s.getIssue(issueId), 'The issue could not be loaded.');
  const tags = useSection(source, issueId, refresh, (s) => s.getIssueTags(issueId), 'The tags could not be loaded.');
  const history = useSection(
    source,
    issueId,
    refresh,
    (s) => s.getIssueHistory(issueId),
    'The history could not be loaded.'
  );
  const events = useSection(
    source,
    issueId,
    refresh,
    (s) => s.listEvents({ queryType: 'events', issue: issueId }),
    'The events could not be loaded.'
  );
  const issue = summary.value;
  const pending = summary.pending || tags.pending || history.pending || events.pending;

  const actions = [
    <Button key="back" icon="arrow-left" variant="secondary" onClick={() => navigate(listPath(search))}>
      Back to issues
    </Button>,
    issue && (
      <LinkButton
        key="bugsink"
        href={issue.bugsinkUrl}
        target="_blank"
        rel="noreferrer"
        icon="external-link-alt"
        variant="secondary"
      >
        Open in Bugsink
      </LinkButton>
    ),
    <RefreshPicker
      key="refresh"
      noIntervalPicker
      isLoading={pending}
      onRefresh={() => setRefresh((count) => count + 1)}
      onIntervalChanged={() => {}}
    />,
  ];

  return (
    <PluginPage pageNav={{ text: issue?.friendlyId ?? issueId }} actions={actions}>
      <NoDataSource choice={choice} requested={requested} />
      <SectionState loading="Loading issue..." state={summary} />
      {!summary.error && (
        <>
          {issue && <IssueSummary issue={issue} timeZone={timeZone} />}
          <PageSection title="Tags" loading="Loading tags..." state={tags}>
            {tags.value && <IssueTags tags={tags.value} />}
          </PageSection>
          <PageSection title="History" loading="Loading history..." state={history}>
            {history.value && <IssueHistory history={history.value} timeZone={timeZone} />}
          </PageSection>
          <PageSection title="Events" loading="Loading events..." state={events}>
            {events.value && (
              <>
                {events.value.notices.map((notice) => (
                  <Alert key={notice} severity="warning" title={notice} />
                ))}
                <EventTable rows={events.value.rows} timeZone={timeZone} />
              </>
            )}
          </PageSection>
        </>
      )}
    </PluginPage>
  );
}
