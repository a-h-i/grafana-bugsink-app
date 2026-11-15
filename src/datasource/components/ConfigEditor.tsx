// SPDX-License-Identifier: GPL-3.0-or-later
import React, { ChangeEvent } from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { DataSourcePicker } from '@grafana/runtime';
import { Field, Input, SecretInput } from '@grafana/ui';
import { BugsinkOptions, BugsinkSecureJsonData } from '../types';

const DEFAULT_LOGS_QUERY = '{service_name="${service}"} | trace_id="${trace_id}"';

type Props = DataSourcePluginOptionsEditorProps<BugsinkOptions, BugsinkSecureJsonData>;

export function ConfigEditor({ options, onOptionsChange }: Props) {
  const { secureJsonFields, secureJsonData } = options;
  const jsonData = options.jsonData ?? {};

  const onUrlChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({ ...options, url: event.target.value });
  };

  const onBrowserUrlChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({ ...options, jsonData: { ...options.jsonData, browserUrl: event.target.value } });
  };

  const setJsonData = (changes: Partial<BugsinkOptions>) => {
    const next = { ...options.jsonData, ...changes };
    for (const key of Object.keys(changes) as Array<keyof BugsinkOptions>) {
      if (next[key] === undefined) {
        delete next[key];
      }
    }
    onOptionsChange({ ...options, jsonData: next });
  };

  const onWindowChange = (event: ChangeEvent<HTMLInputElement>) => {
    const text = event.target.value;
    setJsonData({ linkWindowMinutes: text === '' ? undefined : Number(text) });
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
      <h3 className="page-heading">Trace and log links</h3>
      <Field label="Tempo data source" description="Open trace runs a trace ID query here." htmlFor="bugsink-tempo">
        <DataSourcePicker
          inputId="bugsink-tempo"
          type="tempo"
          noDefault
          width={50}
          current={jsonData.tempoDatasourceUid ?? null}
          onChange={(ds) => setJsonData({ tempoDatasourceUid: ds.uid })}
          onClear={() => setJsonData({ tempoDatasourceUid: undefined })}
        />
      </Field>
      <Field label="Loki data source" description="Open logs runs the logs query here." htmlFor="bugsink-loki">
        <DataSourcePicker
          inputId="bugsink-loki"
          type="loki"
          noDefault
          width={50}
          current={jsonData.lokiDatasourceUid ?? null}
          onChange={(ds) => setJsonData({ lokiDatasourceUid: ds.uid })}
          onClear={() => setJsonData({ lokiDatasourceUid: undefined })}
        />
      </Field>
      <Field
        label="Time window"
        description="Links open this many minutes before and after the event."
        htmlFor="bugsink-link-window"
      >
        <Input
          id="bugsink-link-window"
          aria-label="Time window"
          type="number"
          width={20}
          min={1}
          max={1440}
          suffix="minutes"
          placeholder="15"
          value={jsonData.linkWindowMinutes ?? ''}
          onChange={onWindowChange}
        />
      </Field>
      <Field
        label="Logs query"
        description="LogQL for Open logs. Variables: ${trace_id}, ${service}, ${project}, ${environment}, ${release}, ${tag:<name>}. Put every variable inside double quotes. Values are not escaped for regular expressions. Leave empty for the default."
        htmlFor="bugsink-logs-query"
      >
        <Input
          id="bugsink-logs-query"
          aria-label="Logs query"
          width={50}
          placeholder={DEFAULT_LOGS_QUERY}
          value={jsonData.logsQuery ?? ''}
          onChange={(event) => setJsonData({ logsQuery: event.target.value || undefined })}
        />
      </Field>
      <Field
        label="Service tag"
        description="The event tag that holds the service name for ${service}. When it is empty or the event lacks the tag, the Bugsink project name is used."
        htmlFor="bugsink-service-tag"
      >
        <Input
          id="bugsink-service-tag"
          aria-label="Service tag"
          width={50}
          value={jsonData.serviceTag ?? 'service'}
          onChange={(event) => setJsonData({ serviceTag: event.target.value })}
        />
      </Field>
    </>
  );
}
