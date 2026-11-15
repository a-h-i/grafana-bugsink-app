// SPDX-License-Identifier: GPL-3.0-or-later
import { DataSourcePlugin } from '@grafana/data';
import { ConfigEditor } from './components/ConfigEditor';
import { BugsinkDataSource } from './datasource';
import { BugsinkOptions, BugsinkQuery, BugsinkSecureJsonData } from './types';

export const plugin = new DataSourcePlugin<BugsinkDataSource, BugsinkQuery, BugsinkOptions, BugsinkSecureJsonData>(
  BugsinkDataSource
).setConfigEditor(ConfigEditor);
