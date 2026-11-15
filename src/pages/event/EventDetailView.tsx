// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { css } from '@emotion/css';
import { dateTimeFormat, GrafanaTheme2 } from '@grafana/data';
import { TimeZone } from '@grafana/schema';
import { Alert, Column, InteractiveTable, Text, useStyles2 } from '@grafana/ui';
import { EventDetail, KeyValue } from '../../datasource/types';
import { PAGE_SIZE } from '../issues/IssueTable';
import { Breadcrumbs } from './Breadcrumbs';
import { ExceptionChain } from './StackTrace';

const TIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

interface ContextRow {
  context: string;
  key: string;
  value: string;
}

function getStyles(theme: GrafanaTheme2) {
  return {
    summary: css({
      display: 'grid',
      gridTemplateColumns: 'max-content 1fr',
      gap: theme.spacing(0.5, 3),
      margin: theme.spacing(0, 0, 2),
      dd: { margin: 0, overflowWrap: 'anywhere' },
    }),
    message: css({ overflowWrap: 'anywhere' }),
    note: css({ margin: theme.spacing(0, 0, 1), color: theme.colors.text.secondary }),
    pre: css({ padding: theme.spacing(1), overflowX: 'auto', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }),
  };
}

function Section({ title, children }: { title: string; children: React.ReactNode }): React.JSX.Element {
  return (
    <section>
      <h3>{title}</h3>
      {children}
    </section>
  );
}

function pairColumns(first: string, second: string): Array<Column<KeyValue>> {
  return [
    { id: 'key', header: first, cell: ({ row }) => <span>{row.original.key}</span> },
    { id: 'value', header: second, cell: ({ row }) => <span>{row.original.value}</span> },
  ];
}

const contextColumns: Array<Column<ContextRow>> = [
  { id: 'context', header: 'Context', cell: ({ row }) => <span>{row.original.context}</span> },
  { id: 'key', header: 'Key', cell: ({ row }) => <span>{row.original.key}</span> },
  { id: 'value', header: 'Value', cell: ({ row }) => <span>{row.original.value}</span> },
];

export function EventDetailView({ event, timeZone }: { event: EventDetail; timeZone: TimeZone }): React.JSX.Element {
  const styles = useStyles2(getStyles);
  const rows: Array<[string, string]> = [
    ['Event ID', event.eventId],
    ['Timestamp', dateTimeFormat(event.timestamp, { timeZone, format: TIME_FORMAT })],
    ['Level', event.level],
    ['Environment', event.environment],
    ['Release', event.release],
    ['Transaction', event.transaction],
    ['Trace ID', event.traceId],
  ];
  const shown = rows.filter(([label, value]) => value !== '' || label === 'Event ID');
  const contextRows = event.contexts.flatMap((context) =>
    context.fields.map((field) => ({ context: context.name, ...field }))
  );
  const hasExceptions = event.exceptions.length > 0;
  const hasFrames = event.exceptions.some((e) => e.frames.length > 0);

  return (
    <>
      <dl className={styles.summary}>
        {shown.map(([label, value]) => (
          <React.Fragment key={label}>
            <dt>
              <Text weight="bold">{label}</Text>
            </dt>
            <dd>{value}</dd>
          </React.Fragment>
        ))}
      </dl>
      {event.notices.map((notice, i) => (
        <Alert key={i} severity="warning" title={notice} />
      ))}
      {hasExceptions && (
        <Section title="Exception">
          <ExceptionChain exceptions={event.exceptions} platform={event.platform} />
        </Section>
      )}
      {!hasExceptions && event.message !== '' && (
        <Section title="Message">
          <p className={styles.message}>{event.message}</p>
        </Section>
      )}
      {!hasFrames && (
        <Section title="Stack trace">
          {event.stacktraceMd ? (
            <>
              <p className={styles.note}>
                Bugsink&apos;s own rendering of the stack trace. It can include local variables.
              </p>
              <pre className={styles.pre}>{event.stacktraceMd}</pre>
            </>
          ) : (
            <p>No stack trace.</p>
          )}
        </Section>
      )}
      <Section title="Breadcrumbs">
        <Breadcrumbs breadcrumbs={event.breadcrumbs} timeZone={timeZone} />
      </Section>
      <Section title="Event tags">
        {event.tags.length === 0 ? (
          <p>No tags.</p>
        ) : (
          <InteractiveTable
            columns={pairColumns('Tag', 'Value')}
            data={event.tags}
            getRowId={(_, i) => String(i)}
            pageSize={PAGE_SIZE}
          />
        )}
      </Section>
      <Section title="Contexts">
        {contextRows.length === 0 ? (
          <p>No contexts.</p>
        ) : (
          <InteractiveTable
            columns={contextColumns}
            data={contextRows}
            getRowId={(_, i) => String(i)}
            pageSize={PAGE_SIZE}
          />
        )}
      </Section>
    </>
  );
}
