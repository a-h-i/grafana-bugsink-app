// SPDX-License-Identifier: GPL-3.0-or-later
import React, { KeyboardEvent, useEffect, useMemo, useState } from 'react';
import { QueryEditorProps } from '@grafana/data';
import { Combobox, ComboboxOption, Field, Input, InlineFieldRow, Alert } from '@grafana/ui';
import { BugsinkDataSource } from '../datasource';
import { BugsinkOptions, BugsinkQuery, DEFAULT_QUERY, IssueSort, IssueState, Project, SortOrder } from '../types';

type Props = QueryEditorProps<BugsinkDataSource, BugsinkQuery, BugsinkOptions>;

const ALL_PROJECTS = 0;

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

function errorMessage(error: unknown): string {
  const message = (error as { data?: { message?: string } })?.data?.message;
  return message || 'Could not load the projects.';
}

function projectOptions(projects: Project[], selected: number): Array<ComboboxOption<number>> {
  const sorted = [...projects].sort((a, b) => a.name.localeCompare(b.name));
  const options = [
    { value: ALL_PROJECTS, label: 'All projects' },
    ...sorted.map((p) => ({ value: p.id, label: p.name })),
  ];
  if (selected !== ALL_PROJECTS && !projects.some((p) => p.id === selected)) {
    options.push({ value: selected, label: `Project ${selected}` });
  }
  return options;
}

export function QueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loadError, setLoadError] = useState<string>();
  const [draft, setDraft] = useState<string>();
  const search = draft ?? query.q ?? '';

  useEffect(() => {
    let active = true;
    datasource
      .getProjects()
      .then((list) => active && setProjects(list))
      .catch((error) => active && setLoadError(errorMessage(error)));
    return () => {
      active = false;
    };
  }, [datasource]);

  const project = query.project || ALL_PROJECTS;
  const options = useMemo(() => projectOptions(projects, project), [projects, project]);
  const sortOptions = project === ALL_PROJECTS ? SORT_OPTIONS : [...SORT_OPTIONS, ARRIVAL_ORDER];

  const update = (changes: Partial<BugsinkQuery>) => {
    onChange({ ...query, ...changes });
    onRunQuery();
  };

  const onProjectChange = ({ value }: ComboboxOption<number>) => {
    if (value === ALL_PROJECTS) {
      const { project, ...rest } = query;
      const sort = query.sort === 'digest_order' ? 'last_seen' : query.sort;
      onChange({ ...rest, ...(sort && { sort }) });
      onRunQuery();
    } else {
      update({ project: value });
    }
  };

  const applySearch = () => {
    setDraft(undefined);
    if (search !== (query.q ?? '')) {
      update({ q: search });
    }
  };

  const onSearchKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      applySearch();
    }
  };

  return (
    <>
      {loadError && <Alert severity="error" title={loadError} />}
      <InlineFieldRow>
        <Field label="Project">
          <Combobox
            aria-label="Project"
            width={30}
            options={options}
            value={project}
            onChange={onProjectChange}
          />
        </Field>
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
      <Field label="Search">
        <Input
          aria-label="Search"
          placeholder="environment:production release:1.4.2"
          value={search}
          onChange={(event) => setDraft(event.currentTarget.value)}
          onBlur={applySearch}
          onKeyDown={onSearchKeyDown}
        />
      </Field>
    </>
  );
}
