// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { Alert, EmptyState, TextLink } from '@grafana/ui';
import { DataSourceChoice } from './useDataSource';

export function NoDataSource({
  choice,
  requested,
}: {
  choice: DataSourceChoice;
  requested?: string;
}): React.JSX.Element | null {
  const { instances, uid, unknown, failed } = choice;
  if (instances.length === 0) {
    return (
      <EmptyState variant="call-to-action" message="No Bugsink data source yet.">
        <TextLink href="/connections/datasources/new">Add a Bugsink data source</TextLink>
      </EmptyState>
    );
  }
  if (unknown) {
    return <Alert severity="error" title={`The data source "${requested}" was not found.`} />;
  }
  if (failed) {
    const name = instances.find((instance) => instance.uid === uid)?.name;
    return <Alert severity="error" title={`The data source "${name}" could not be loaded.`} />;
  }
  return null;
}
