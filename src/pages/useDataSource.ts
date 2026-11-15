// SPDX-License-Identifier: GPL-3.0-or-later
import { useEffect, useState } from 'react';
import { DataSourceInstanceSettings } from '@grafana/data';
import { getDataSourceSrv } from '@grafana/runtime';
import { BugsinkDataSource } from '../datasource/datasource';

const DATASOURCE_TYPE = 'ahmedismail-bugsink-datasource';

export interface DataSourceChoice {
  instances: DataSourceInstanceSettings[];
  uid?: string;
  unknown: boolean;
  source?: BugsinkDataSource;
  failed: boolean;
}

function defaultInstance(instances: DataSourceInstanceSettings[]): DataSourceInstanceSettings | undefined {
  return (
    instances.find((instance) => instance.isDefault) ?? [...instances].sort((a, b) => a.name.localeCompare(b.name))[0]
  );
}

export function useDataSource(requested: string | undefined, attempt: number): DataSourceChoice {
  const instances = getDataSourceSrv().getList({ pluginId: DATASOURCE_TYPE });
  const unknown = requested !== undefined && !instances.some((instance) => instance.uid === requested);
  const uid = requested ?? defaultInstance(instances)?.uid;
  const [loaded, setLoaded] = useState<{ uid: string; source?: BugsinkDataSource }>();

  useEffect(() => {
    if (!uid || unknown) {
      return;
    }
    let active = true;
    getDataSourceSrv()
      .get(uid)
      .then((instance) => active && setLoaded({ uid, source: instance as BugsinkDataSource }))
      .catch(() => active && setLoaded({ uid }));
    return () => {
      active = false;
    };
  }, [uid, unknown, attempt]);

  const mine = loaded && loaded.uid === uid ? loaded : undefined;
  return { instances, uid, unknown, source: mine?.source, failed: mine !== undefined && !mine.source };
}
