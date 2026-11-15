// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { QueryEditorProps } from '@grafana/data';
import { Field, RadioButtonGroup } from '@grafana/ui';
import { BugsinkDataSource } from '../datasource';
import { BugsinkOptions, BugsinkQuery, QueryType } from '../types';
import { EventsQueryEditor } from './EventsQueryEditor';
import { EventCountQueryEditor } from './EventCountQueryEditor';
import { IssueCountQueryEditor } from './IssueCountQueryEditor';
import { IssuesQueryEditor } from './IssuesQueryEditor';
import { NewIssuesQueryEditor } from './NewIssuesQueryEditor';
import { VolumeQueryEditor } from './VolumeQueryEditor';

type Props = QueryEditorProps<BugsinkDataSource, BugsinkQuery, BugsinkOptions>;

const QUERY_TYPES: Array<{ value: QueryType; label: string }> = [
  { value: 'issues', label: 'Issues' },
  { value: 'events', label: 'Events' },
  { value: 'volume', label: 'Event volume' },
  { value: 'newIssues', label: 'New issues' },
  { value: 'issueCount', label: 'Issue count' },
  { value: 'eventCount', label: 'Event count' },
];

export function QueryEditor(props: Props) {
  const { query, onChange, onRunQuery } = props;
  const queryType: QueryType = QUERY_TYPES.some((t) => t.value === query.queryType)
    ? (query.queryType as QueryType)
    : 'issues';

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
      {queryType === 'events' && <EventsQueryEditor {...props} />}
      {queryType === 'volume' && <VolumeQueryEditor {...props} />}
      {queryType === 'issues' && <IssuesQueryEditor {...props} />}
      {queryType === 'newIssues' && <NewIssuesQueryEditor {...props} />}
      {queryType === 'issueCount' && <IssueCountQueryEditor {...props} />}
      {queryType === 'eventCount' && <EventCountQueryEditor {...props} />}
    </>
  );
}
