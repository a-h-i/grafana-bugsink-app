// SPDX-License-Identifier: GPL-3.0-or-later
import { AppPlugin } from '@grafana/data';
import { IssuesPage } from './pages/IssuesPage';

export const plugin = new AppPlugin().setRootPage(IssuesPage);
