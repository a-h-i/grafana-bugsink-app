// SPDX-License-Identifier: GPL-3.0-or-later
import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export interface BugsinkQuery extends DataQuery {}

export interface BugsinkOptions extends DataSourceJsonData {}

export interface BugsinkSecureJsonData {
  apiToken?: string;
}
