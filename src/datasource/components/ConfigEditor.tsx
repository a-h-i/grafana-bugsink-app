// SPDX-License-Identifier: GPL-3.0-or-later
import React, { ChangeEvent } from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { Field, Input, SecretInput } from '@grafana/ui';
import { BugsinkOptions, BugsinkSecureJsonData } from '../types';

type Props = DataSourcePluginOptionsEditorProps<BugsinkOptions, BugsinkSecureJsonData>;

export function ConfigEditor({ options, onOptionsChange }: Props) {
  const { secureJsonFields, secureJsonData } = options;

  const onUrlChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({ ...options, url: event.target.value });
  };

  const onBrowserUrlChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({ ...options, jsonData: { ...options.jsonData, browserUrl: event.target.value } });
  };

  const onTokenChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({ ...options, secureJsonData: { ...secureJsonData, apiToken: event.target.value } });
  };

  const onTokenReset = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: { ...secureJsonFields, apiToken: false },
      secureJsonData: { ...secureJsonData, apiToken: '' },
    });
  };

  return (
    <>
      <Field label="Bugsink URL" description="The address of Bugsink itself, without /api." htmlFor="bugsink-url">
        <Input
          id="bugsink-url"
          aria-label="Bugsink URL"
          width={50}
          placeholder="https://bugsink.example.com"
          value={options.url ?? ''}
          onChange={onUrlChange}
        />
      </Field>
      <Field
        label="Browser URL"
        description="The address of Bugsink as your browser reaches it, used for links to issues. Leave empty when it is the same as the Bugsink URL."
        htmlFor="bugsink-browser-url"
      >
        <Input
          id="bugsink-browser-url"
          aria-label="Browser URL"
          width={50}
          placeholder="https://bugsink.example.com"
          value={options.jsonData?.browserUrl ?? ''}
          onChange={onBrowserUrlChange}
        />
      </Field>
      <Field
        label="API token"
        description="A Bugsink API token with the capabilities projects:read, issues:read and events:read. A project-bound token works: the plugin shows only that token's project."
        htmlFor="bugsink-api-token"
      >
        <SecretInput
          id="bugsink-api-token"
          aria-label="API token"
          width={50}
          isConfigured={Boolean(secureJsonFields?.apiToken)}
          value={secureJsonData?.apiToken ?? ''}
          onChange={onTokenChange}
          onReset={onTokenReset}
        />
      </Field>
    </>
  );
}
