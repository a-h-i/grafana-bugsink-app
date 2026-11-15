// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { getDataSourceSrv, PluginPage } from '@grafana/runtime';
import { TextLink } from '@grafana/ui';

const DATASOURCE_TYPE = 'ahmedismail-bugsink-datasource';

export function Home(): React.JSX.Element {
  const instances = getDataSourceSrv()
    .getList({ type: DATASOURCE_TYPE })
    .filter((instance) => instance.type === DATASOURCE_TYPE);

  return (
    <PluginPage>
      <p>Issue browsing arrives in a later version. For now, connect Grafana to Bugsink with a data source.</p>
      {instances.length === 0 ? (
        <>
          <p>No Bugsink data source yet.</p>
          <TextLink href="/connections/datasources/new">Add a Bugsink data source</TextLink>
        </>
      ) : (
        <ul>
          {instances.map((instance) => (
            <li key={instance.uid}>
              <TextLink href={`/connections/datasources/edit/${instance.uid}`}>{instance.name}</TextLink>
            </li>
          ))}
        </ul>
      )}
    </PluginPage>
  );
}
