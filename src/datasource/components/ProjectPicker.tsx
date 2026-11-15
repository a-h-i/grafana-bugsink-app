// SPDX-License-Identifier: GPL-3.0-or-later
import React, { useEffect, useMemo, useState } from 'react';
import { Combobox, ComboboxOption, Field } from '@grafana/ui';
import { BugsinkDataSource } from '../datasource';
import { Project } from '../types';

interface Props {
  datasource: BugsinkDataSource;
  value?: number;
  onChange: (project?: number) => void;
  emptyLabel?: string;
  description?: string;
}

const ALL_PROJECTS = 0;

function errorMessage(error: unknown): string {
  const message = (error as { data?: { message?: string } })?.data?.message;
  return message || 'Could not load the projects.';
}

function projectOptions(projects: Project[], selected: number, emptyLabel: string): Array<ComboboxOption<number>> {
  const sorted = [...projects].sort((a, b) => a.name.localeCompare(b.name));
  const options = [{ value: ALL_PROJECTS, label: emptyLabel }, ...sorted.map((p) => ({ value: p.id, label: p.name }))];
  if (selected !== ALL_PROJECTS && !projects.some((p) => p.id === selected)) {
    options.push({ value: selected, label: `Project ${selected}` });
  }
  return options;
}

export function ProjectPicker({ datasource, value, onChange, emptyLabel = 'All projects', description }: Props) {
  const [loaded, setLoaded] = useState<{ source: BugsinkDataSource; projects: Project[]; error?: string }>();
  const current = loaded?.source === datasource ? loaded : undefined;
  const projects = useMemo(() => current?.projects ?? [], [current]);
  const selected = value || ALL_PROJECTS;
  const options = useMemo(() => projectOptions(projects, selected, emptyLabel), [projects, selected, emptyLabel]);

  useEffect(() => {
    let active = true;
    datasource
      .getProjects()
      .then((list) => active && setLoaded({ source: datasource, projects: list }))
      .catch((error) => active && setLoaded({ source: datasource, projects: [], error: errorMessage(error) }));
    return () => {
      active = false;
    };
  }, [datasource]);

  return (
    <Field label="Project" description={description} invalid={Boolean(current?.error)} error={current?.error}>
      <Combobox
        aria-label="Project"
        width={30}
        options={options}
        value={selected}
        onChange={(option) => onChange(option.value === ALL_PROJECTS ? undefined : option.value)}
      />
    </Field>
  );
}
