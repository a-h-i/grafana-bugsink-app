// SPDX-License-Identifier: GPL-3.0-or-later
import { AppPlugin } from '@grafana/data';
import { App } from './pages/App';

export const plugin = new AppPlugin().setRootPage(App);
