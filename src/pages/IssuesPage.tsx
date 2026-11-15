// SPDX-License-Identifier: GPL-3.0-or-later
import React, { useEffect, useMemo, useState } from 'react';
import { TimeRange, toURLRange } from '@grafana/data';
import { config, PluginPage } from '@grafana/runtime';
import { Alert, Combobox, Field, LoadingBar, LoadingPlaceholder, RefreshPicker, TimeRangePicker } from '@grafana/ui';
import { useLocation, useNavigate } from 'react-router-dom';
import { BugsinkDataSource } from '../datasource/datasource';
import { IssuesQueryEditor } from '../datasource/components/IssuesQueryEditor';
import { BugsinkQuery, IssueList } from '../datasource/types';
import { IssueTable } from './issues/IssueTable';
import { ListState, listSearch, readListState, writeListState } from './issues/listState';
import { NoDataSource } from './NoDataSource';
import { issuePath } from './routes';
import { useDataSource } from './useDataSource';
import { shiftTimeRange, toTimeRange, zoomOutTimeRange } from './issues/timeRange';

type Outcome = { source: BugsinkDataSource; key: string } & ({ list: IssueList } | { error: string });

const keyOf = (canonical: string, refresh: number) => `${canonical}#${refresh}`;

export function IssuesPage(): React.JSX.Element {
  const location = useLocation();
  const navigate = useNavigate();
  const urlState = useMemo(() => readListState(new URLSearchParams(location.search)), [location.search]);
  const requested = urlState.ds;
  const [refresh, setRefresh] = useState(0);
  const choice = useDataSource(requested, refresh);
  const { instances, uid, unknown, source, failed } = choice;
  const range = toTimeRange(urlState.from, urlState.to);
  const urlRange = toURLRange(range.raw);
  const state: ListState = { ...urlState, ds: uid, from: String(urlRange.from), to: String(urlRange.to) };

  const [timeZone, setTimeZone] = useState(config.bootData.user.timezone);
  const [outcome, setOutcome] = useState<Outcome>();

  const canonical = writeListState(state).toString();
  const key = keyOf(canonical, refresh);
  const wanted = listSearch(state, location.search);

  useEffect(() => {
    if (uid && wanted !== location.search) {
      navigate({ search: wanted }, { replace: true });
    }
  }, [uid, wanted, location.search, navigate]);

  useEffect(() => {
    if (!source) {
      return;
    }
    let active = true;
    const { query, from, to } = readListState(new URLSearchParams(canonical));
    const requestKey = keyOf(canonical, refresh);
    source
      .listIssues(query, toTimeRange(from, to))
      .then((list): Outcome => ({ source, key: requestKey, list }))
      .catch((error): Outcome => ({
        source,
        key: requestKey,
        error: error?.message || 'The issues could not be loaded.',
      }))
      .then((result) => active && setOutcome(result));
    return () => {
      active = false;
    };
  }, [source, canonical, refresh]);

  if (instances.length === 0) {
    return (
      <PluginPage>
        <NoDataSource choice={choice} />
      </PluginPage>
    );
  }

  const write = (next: ListState) => navigate({ search: listSearch(next, location.search) }, { replace: true });
  const writeRange = (range: TimeRange) => {
    const { from, to } = toURLRange(range.raw);
    write({ ...state, from: String(from), to: String(to) });
  };
  const onQueryChange = (query: BugsinkQuery) => write({ ...state, query: { ...query, refId: 'issues' } });
  const onInstanceChange = (next: string) => {
    const { project, ...rest } = state.query;
    const sort = rest.sort === 'digest_order' ? 'last_seen' : rest.sort;
    write({ ...state, ds: next, query: { ...rest, sort } });
  };

  const current = outcome?.source === source ? outcome : undefined;

  const actions = [
    <TimeRangePicker
      key="range"
      value={range}
      timeZone={timeZone}
      onChange={writeRange}
      onChangeTimeZone={setTimeZone}
      onMoveBackward={() => writeRange(shiftTimeRange(range, -1))}
      onMoveForward={() => writeRange(shiftTimeRange(range, 1))}
      onZoom={() => writeRange(zoomOutTimeRange(range))}
    />,
    <RefreshPicker
      key="refresh"
      noIntervalPicker
      isLoading={source !== undefined && outcome?.key !== key}
      onRefresh={() => setRefresh((count) => count + 1)}
      onIntervalChanged={() => {}}
    />,
  ];

  return (
    <PluginPage actions={actions}>
      <Field label="Data source">
        <Combobox
          aria-label="Data source"
          width={30}
          options={instances.map((instance) => ({ value: instance.uid, label: instance.name }))}
          value={unknown ? null : uid}
          onChange={({ value }) => onInstanceChange(value)}
        />
      </Field>
      <NoDataSource choice={choice} requested={requested} />
      {source && (
        <IssuesQueryEditor datasource={source} query={state.query} onChange={onQueryChange} onRunQuery={() => {}} />
      )}
      {current && current.key !== key && <LoadingBar width={600} />}
      {current && 'error' in current && <Alert severity="error" title={current.error} />}
      {current && 'list' in current && current.list.notice && <Alert severity="warning" title={current.list.notice} />}
      {current && 'list' in current && (
        <IssueTable
          rows={current.list.rows}
          timeZone={timeZone}
          issueHref={(row) => issuePath(row.id, location.search)}
        />
      )}
      {!unknown && !failed && !current && <LoadingPlaceholder text="Loading issues..." />}
    </PluginPage>
  );
}
