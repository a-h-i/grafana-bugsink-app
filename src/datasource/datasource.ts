// SPDX-License-Identifier: GPL-3.0-or-later
import { DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';
import { BugsinkOptions, BugsinkQuery, DEFAULT_QUERY, Project } from './types';

export class BugsinkDataSource extends DataSourceWithBackend<BugsinkQuery, BugsinkOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<BugsinkOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(): Partial<BugsinkQuery> {
    return DEFAULT_QUERY;
  }

  applyTemplateVariables(query: BugsinkQuery, scopedVars: ScopedVars): BugsinkQuery {
    return { ...query, q: getTemplateSrv().replace(query.q, scopedVars) };
  }

  getProjects(): Promise<Project[]> {
    return this.getResource('projects');
  }
}
