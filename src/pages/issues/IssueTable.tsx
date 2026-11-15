// SPDX-License-Identifier: GPL-3.0-or-later
import React, { useMemo } from 'react';
import { TimeZone } from '@grafana/schema';
import { CellProps, Column, EmptyState, InteractiveTable, LinkButton, Stack } from '@grafana/ui';
import { Link } from 'react-router-dom';
import { IssueRow, TriageAction } from '../../datasource/types';
import { formatTime, STATUS_LABELS } from '../format';
import { ACTION_LABELS, TriageButtons, triageActions } from '../TriageButtons';
import { IssueTableContext, useIssueTable, VolumeSource } from './IssueTableContext';
import { SparklineCell } from './SparklineCell';

export const PAGE_SIZE = 25;

const LIST_ACTIONS: TriageAction[] = ['resolve', 'mute'];

interface IssueTableProps {
  rows: IssueRow[];
  timeZone: TimeZone;
  issueHref: (row: IssueRow) => string;
  source: VolumeSource;
  triage?: {
    busyIds?: ReadonlySet<string>;
    disabled?: boolean;
    onAction: (row: IssueRow, action: TriageAction) => void;
  };
}

function RowIssue({ row }: { row: IssueRow }): React.JSX.Element {
  const { issueHref } = useIssueTable();
  return (
    <Stack alignItems="center" gap={0.5}>
      <Link to={issueHref(row)}>{row.friendlyId}</Link>
      {row.url && (
        <LinkButton
          href={row.url}
          target="_blank"
          rel="noreferrer"
          icon="external-link-alt"
          variant="secondary"
          fill="text"
          size="sm"
          aria-label={`Open ${row.friendlyId} in Bugsink`}
        />
      )}
    </Stack>
  );
}

function RowActions({ row }: { row: IssueRow }): React.JSX.Element {
  const { busyIds, disabled, onAction } = useIssueTable();
  return (
    <TriageButtons
      actions={triageActions(row.status).filter((action) => LIST_ACTIONS.includes(action))}
      busy={disabled || (busyIds?.has(row.id) ?? false)}
      name={(action) => `${ACTION_LABELS[action]} ${row.friendlyId}`}
      onAction={(action) => onAction?.(row, action)}
    />
  );
}

export function IssueTable({ rows, timeZone, issueHref, source, triage }: IssueTableProps): React.JSX.Element {
  const withActions = triage !== undefined;
  const columns = useMemo<Array<Column<IssueRow>>>(() => {
    const time = (value: number) => formatTime(value, timeZone);
    const list: Array<Column<IssueRow>> = [
      { id: 'friendlyId', header: 'Issue', cell: ({ row }: CellProps<IssueRow>) => <RowIssue row={row.original} /> },
      { id: 'type', header: 'Type', cell: ({ row }) => <span>{row.original.type}</span> },
      { id: 'message', header: 'Message', cell: ({ row }) => <span>{row.original.message}</span> },
      { id: 'transaction', header: 'Transaction', cell: ({ row }) => <span>{row.original.transaction}</span> },
      { id: 'project', header: 'Project', cell: ({ row }) => <span>{row.original.project}</span> },
      { id: 'events', header: 'Events', cell: ({ row }) => <span>{row.original.events}</span> },
      { id: 'volume', header: 'Last 24 hours', cell: ({ row }) => <SparklineCell id={row.original.id} /> },
      { id: 'firstSeen', header: 'First seen', cell: ({ row }) => <span>{time(row.original.firstSeen)}</span> },
      { id: 'lastSeen', header: 'Last seen', cell: ({ row }) => <span>{time(row.original.lastSeen)}</span> },
      { id: 'status', header: 'Status', cell: ({ row }) => <span>{STATUS_LABELS[row.original.status]}</span> },
    ];
    if (withActions) {
      list.push({
        id: 'actions',
        header: 'Actions',
        cell: ({ row }: CellProps<IssueRow>) => <RowActions row={row.original} />,
      });
    }
    return list;
  }, [timeZone, withActions]);

  const busyIds = triage?.busyIds;
  const disabled = triage?.disabled;
  const onAction = triage?.onAction;
  const context = useMemo(
    () => ({ source, issueHref, busyIds, disabled, onAction }),
    [source, issueHref, busyIds, disabled, onAction]
  );

  if (rows.length === 0) {
    return <EmptyState variant="not-found" message="No issues match these filters in this time range." />;
  }

  return (
    <IssueTableContext.Provider value={context}>
      <InteractiveTable columns={columns} data={rows} getRowId={(row) => row.id} pageSize={PAGE_SIZE} />
    </IssueTableContext.Provider>
  );
}
