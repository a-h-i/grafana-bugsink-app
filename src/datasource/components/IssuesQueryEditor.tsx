// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { QueryEditorProps } from '@grafana/data';
import { Combobox, ComboboxOption, Field, InlineFieldRow } from '@grafana/ui';
import { BugsinkDataSource } from '../datasource';
import { BugsinkOptions, BugsinkQuery, DEFAULT_QUERY, IssueSort, IssueState, SortOrder } from '../types';
import { CommitInput } from './CommitInput';
import { ProjectPicker } from './ProjectPicker';

type Props = QueryEditorProps<BugsinkDataSource, BugsinkQuery, BugsinkOptions>;

const STATE_OPTIONS: Array<ComboboxOption<IssueState>> = [
  { value: 'all', label: 'All' },
  { value: 'open', label: 'Open' },
  { value: 'unresolved', label: 'Unresolved' },
  { value: 'resolved', label: 'Resolved' },
  { value: 'muted', label: 'Muted' },
];

const SORT_OPTIONS: Array<ComboboxOption<IssueSort>> = [
  { value: 'last_seen', label: 'Last seen' },
  { value: 'digested_event_count', label: 'Event count' },
];

const ARRIVAL_ORDER: ComboboxOption<IssueSort> = { value: 'digest_order', label: 'Arrival order' };

const ORDER_OPTIONS: Array<ComboboxOption<SortOrder>> = [
  { value: 'desc', label: 'Descending' },
  { value: 'asc', label: 'Ascending' },
];

export function IssuesQueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const sortOptions = query.project ? [...SORT_OPTIONS, ARRIVAL_ORDER] : SORT_OPTIONS;

  const update = (changes: Partial<BugsinkQuery>) => {
    onChange({ ...query, ...changes });
    onRunQuery();
  };

  const onProjectChange = (project?: number) => {
    if (project === undefined) {
      const { project: _removed, ...rest } = query;
      const sort = query.sort === 'digest_order' ? 'last_seen' : query.sort;
      onChange({ ...rest, ...(sort && { sort }) });
      onRunQuery();
    } else {
      update({ project });
    }
  };

  return (
    <>
      <InlineFieldRow>
        <ProjectPicker datasource={datasource} value={query.project} onChange={onProjectChange} />
        <Field label="State">
          <Combobox
            aria-label="State"
            width={20}
            options={STATE_OPTIONS}
            value={query.state ?? DEFAULT_QUERY.state}
            onChange={({ value }) => update({ state: value })}
          />
        </Field>
        <Field label="Sort by">
          <Combobox
            aria-label="Sort by"
            width={20}
            options={sortOptions}
            value={query.sort ?? DEFAULT_QUERY.sort}
            onChange={({ value }) => update({ sort: value })}
          />
        </Field>
        <Field label="Order">
          <Combobox
            aria-label="Order"
            width={20}
            options={ORDER_OPTIONS}
            value={query.order ?? DEFAULT_QUERY.order}
            onChange={({ value }) => update({ order: value })}
          />
        </Field>
      </InlineFieldRow>
      <CommitInput
        label="Search"
        value={query.q}
        placeholder="environment:production release:1.4.2"
        onCommit={(q) => onChange({ ...query, q })}
        onRunQuery={onRunQuery}
      />
    </>
  );
}
