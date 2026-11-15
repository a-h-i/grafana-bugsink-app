// SPDX-License-Identifier: GPL-3.0-or-later
import React, { useEffect, useMemo, useState } from 'react';
import { FieldSparkline, FieldType } from '@grafana/data';
import { Icon, Sparkline, Tooltip, useTheme2 } from '@grafana/ui';
import { VolumeSeries } from '../../datasource/types';
import { useIssueTable, VolumeSource } from './IssueTableContext';

type Answer = { source: VolumeSource; id: string } & ({ series: VolumeSeries } | { error: string });

const WIDTH = 120;
const HEIGHT = 24;
const BOX: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: WIDTH,
  height: HEIGHT,
  overflow: 'hidden',
};

function toSparkline({ times, counts }: VolumeSeries): FieldSparkline {
  const max = Math.max(...counts);
  return {
    x: { name: 'time', type: FieldType.time, config: {}, values: times },
    y: {
      name: 'events',
      type: FieldType.number,
      config: {},
      values: counts,
      state: { range: { min: 0, max, delta: max } },
    },
  };
}

export function SparklineCell({ id }: { id: string }): React.JSX.Element {
  const { source } = useIssueTable();
  const theme = useTheme2();
  const [answer, setAnswer] = useState<Answer>();

  useEffect(() => {
    const controller = new AbortController();
    source.issueVolume(id, controller.signal).then(
      (series) => !controller.signal.aborted && setAnswer({ source, id, series }),
      (error) =>
        !controller.signal.aborted &&
        setAnswer({ source, id, error: error?.message || 'The event volume could not be loaded.' })
    );
    return () => controller.abort();
  }, [source, id]);

  const current = answer?.source === source && answer.id === id ? answer : undefined;
  const sparkline = useMemo(() => (current && 'series' in current ? toSparkline(current.series) : undefined), [current]);

  if (!current) {
    return (
      <span role="img" aria-label="Loading the last 24 hours" style={BOX}>
        <Icon name="spinner" />
      </span>
    );
  }
  if ('error' in current) {
    const label = `Events per hour unavailable: ${current.error}`;
    return (
      <Tooltip content={label}>
        <span role="img" aria-label={label} tabIndex={0} style={BOX}>
          <Icon name="exclamation-triangle" />
        </span>
      </Tooltip>
    );
  }
  if (current.series.total === 0) {
    return (
      <span role="img" aria-label="No events in the last 24 hours" style={BOX}>
        No events
      </span>
    );
  }
  const label = `${current.series.total} events in the last 24 hours`;
  return (
    <span role="img" aria-label={label} title={label} style={BOX}>
      {sparkline && <Sparkline width={WIDTH} height={HEIGHT} theme={theme} sparkline={sparkline} />}
    </span>
  );
}
