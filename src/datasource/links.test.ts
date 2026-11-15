// SPDX-License-Identifier: GPL-3.0-or-later
import { createDataFrame, DataLink, FieldType } from '@grafana/data';
import { exploreHref, withLinkRanges } from './links';
import { ExploreLink } from './types';

const FROM = '2026-11-15T09:45:00Z';
const TO = '2026-11-15T10:20:00Z';
const EXPR = '{service_name="sample-python"} | trace_id="abc"';

const logsLink: ExploreLink = {
  title: 'Open logs',
  internal: {
    datasourceUid: 'loki',
    query: { refId: 'A', queryType: 'range', expr: EXPR },
    timeRange: { from: FROM, to: TO },
  },
};

function frame() {
  return createDataFrame({
    fields: [
      { name: 'timestamp', type: FieldType.time, values: [1000] },
      {
        name: 'trace_id',
        type: FieldType.string,
        values: ['abc'],
        config: {
          links: [
            {
              title: 'Open trace',
              url: '',
              internal: { datasourceUid: 'tempo', query: {}, timeRange: { from: FROM, to: TO } },
            },
            { title: 'Plain', url: 'http://example.com' },
            { title: 'No range', url: '', internal: { datasourceUid: 'tempo', query: {} } },
          ] as unknown as DataLink[],
        },
      },
    ],
  });
}

describe('withLinkRanges', () => {
  it('copies the link time range to the range Grafana reads', () => {
    const [out] = withLinkRanges([frame()]);

    const links = out.fields[1].config.links!;
    expect(links[0].internal!.range!.raw.from.valueOf()).toBe(Date.parse(FROM));
    expect(links[0].internal!.range!.raw.to.valueOf()).toBe(Date.parse(TO));
    expect(links[0].internal!.range!.from.valueOf()).toBe(Date.parse(FROM));
    expect(links[0].internal!.range!.to.valueOf()).toBe(Date.parse(TO));
  });

  it('leaves links without a time range and other fields untouched', () => {
    const input = frame();
    const [out] = withLinkRanges([input]);

    const links = out.fields[1].config.links!;
    expect(links[1]).toEqual(input.fields[1].config.links![1]);
    expect(links[2].internal!.range).toBeUndefined();
    expect(out.fields[0]).toBe(input.fields[0]);
  });

  it('does not change its input', () => {
    const input = frame();
    withLinkRanges([input]);

    expect(input.fields[1].config.links![0].internal!.range).toBeUndefined();
  });
});

describe('exploreHref', () => {
  it('opens Explore on the link data source, query and range', () => {
    const href = exploreHref(logsLink);

    expect(href.startsWith('/explore?left=')).toBe(true);
    const state = JSON.parse(decodeURIComponent(href.slice('/explore?left='.length)));
    expect(state.datasource).toBe('loki');
    expect(state.queries[0].expr).toBe(EXPR);
    expect(state.range).toEqual({ from: String(Date.parse(FROM)), to: String(Date.parse(TO)) });
  });
});
