// SPDX-License-Identifier: GPL-3.0-or-later
import { AppPlugin } from '@grafana/data';
import { Home } from './pages/Home';

export const plugin = new AppPlugin().setRootPage(Home);
