// SPDX-License-Identifier: GPL-3.0-or-later
import { DataSourceInstanceSettings } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';
import { BugsinkOptions, BugsinkQuery } from './types';

export class BugsinkDataSource extends DataSourceWithBackend<BugsinkQuery, BugsinkOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<BugsinkOptions>) {
    super(instanceSettings);
  }
}
