// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { QueryEditorProps } from '@grafana/data';
import { Field, RadioButtonGroup } from '@grafana/ui';
import { BugsinkDataSource } from '../datasource';
import { BugsinkOptions, BugsinkQuery, QueryType } from '../types';
import { EventsQueryEditor } from './EventsQueryEditor';
import { IssuesQueryEditor } from './IssuesQueryEditor';

type Props = QueryEditorProps<BugsinkDataSource, BugsinkQuery, BugsinkOptions>;

const QUERY_TYPES: Array<{ value: QueryType; label: string }> = [
  { value: 'issues', label: 'Issues' },
  { value: 'events', label: 'Events' },
];

export function QueryEditor(props: Props) {
  const { query, onChange, onRunQuery } = props;
  const queryType: QueryType = query.queryType === 'events' ? 'events' : 'issues';

  const onTypeChange = (next: QueryType) => {
    const sort = next === 'issues' && !query.project && query.sort === 'digest_order' ? 'last_seen' : query.sort;
    onChange({ ...query, queryType: next, ...(sort && { sort }) });
    onRunQuery();
  };

  return (
    <>
      <Field label="Query type">
        <RadioButtonGroup aria-label="Query type" options={QUERY_TYPES} value={queryType} onChange={onTypeChange} />
      </Field>
      {queryType === 'events' ? <EventsQueryEditor {...props} /> : <IssuesQueryEditor {...props} />}
    </>
  );
}
