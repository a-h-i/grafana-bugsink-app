// SPDX-License-Identifier: GPL-3.0-or-later
import CopyWebpackPlugin from 'copy-webpack-plugin';
import type { Configuration } from 'webpack';
import grafanaConfig, { type Env } from './.config/webpack/webpack.config';

// The nested data source's logo is not among the files the base config copies.
const config = async (env: Env): Promise<Configuration> => {
  const base = await grafanaConfig(env);
  const logo = new CopyWebpackPlugin({ patterns: [{ from: 'datasource/img', to: 'datasource/img' }] });
  return { ...base, plugins: [...(base.plugins ?? []), logo] };
};

export default config;
