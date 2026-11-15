// SPDX-License-Identifier: GPL-3.0-or-later
import { DataFrame, DataLink, dateTime, Field, mapInternalLinkToExplore, TimeRange } from '@grafana/data';
import { ExploreLink } from './types';

export function linkRange(timeRange: { from: string; to: string }): TimeRange {
  const from = dateTime(timeRange.from);
  const to = dateTime(timeRange.to);
  return { from, to, raw: { from, to } };
}

function withRange(link: DataLink): DataLink {
  const timeRange = (link.internal as { timeRange?: { from: string; to: string } } | undefined)?.timeRange;
  if (!link.internal || !timeRange) {
    return link;
  }
  return { ...link, internal: { ...link.internal, range: linkRange(timeRange) } };
}

export function withLinkRanges(frames: DataFrame[]): DataFrame[] {
  return frames.map((frame) => ({
    ...frame,
    fields: frame.fields.map((field) =>
      field.config.links ? { ...field, config: { ...field.config, links: field.config.links.map(withRange) } } : field
    ),
  }));
}

export function exploreHref(link: ExploreLink): string {
  const { datasourceUid, query, timeRange } = link.internal;
  const internalLink = { datasourceUid, datasourceName: '', query: { ...query } };
  return mapInternalLinkToExplore({
    link: { title: link.title, url: '', internal: internalLink },
    internalLink,
    scopedVars: {},
    range: linkRange(timeRange),
    field: {} as Field,
    replaceVariables: (value) => value,
  }).href;
}
