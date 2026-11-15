// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Badge, ControlledCollapse, useStyles2 } from '@grafana/ui';
import { EventException, StackFrame } from '../../datasource/types';
import { chainSeparator, displayExceptions, frameGroups, frameTitle } from './displayOrder';

function getStyles(theme: GrafanaTheme2) {
  return {
    exception: css({ marginBottom: theme.spacing(2) }),
    separator: css({ margin: theme.spacing(2, 0), color: theme.colors.text.secondary }),
    value: css({ margin: theme.spacing(0, 0, 1), overflowWrap: 'anywhere' }),
    frame: css({ margin: theme.spacing(1, 0) }),
    inApp: css({ fontWeight: theme.typography.fontWeightBold }),
    title: css({ fontFamily: theme.typography.fontFamilyMonospace, overflowWrap: 'anywhere' }),
    source: css({ margin: theme.spacing(0.5, 0, 0), padding: theme.spacing(1), overflowX: 'auto' }),
    mark: css({ display: 'block', background: theme.colors.warning.transparent, color: 'inherit' }),
  };
}

function Source({ frame }: { frame: StackFrame }): React.JSX.Element | null {
  const styles = useStyles2(getStyles);
  if (frame.contextLine === null) {
    return null;
  }
  const start = frame.lineno === null ? null : frame.lineno - frame.preContext.length;
  const numbered = (code: string, index: number) => (start === null ? code : `${start + index} | ${code}`);
  const markIndex = frame.preContext.length;
  return (
    <pre className={styles.source}>
      {frame.preContext.map((code, i) => (
        <div key={`pre-${i}`}>{numbered(code, i)}</div>
      ))}
      <mark className={styles.mark}>{numbered(frame.contextLine, markIndex)}</mark>
      {frame.postContext.map((code, i) => (
        <div key={`post-${i}`}>{numbered(code, markIndex + 1 + i)}</div>
      ))}
    </pre>
  );
}

function Frame({ frame, withSource }: { frame: StackFrame; withSource: boolean }): React.JSX.Element {
  const styles = useStyles2(getStyles);
  return (
    <div className={styles.frame}>
      <div className={frame.inApp ? `${styles.title} ${styles.inApp}` : styles.title}>{frameTitle(frame)}</div>
      {withSource && <Source frame={frame} />}
    </div>
  );
}

function Frames({ frames }: { frames: StackFrame[] }): React.JSX.Element {
  if (frames.length === 0) {
    return <p>No frames.</p>;
  }
  return (
    <>
      {frameGroups(frames).map((group, i) =>
        group.kind === 'frame' ? (
          <Frame key={i} frame={group.frame} withSource />
        ) : (
          <ControlledCollapse
            key={i}
            label={`${group.frames.length} other frame${group.frames.length === 1 ? '' : 's'}`}
          >
            {group.frames.map((frame, j) => (
              <Frame key={j} frame={frame} withSource={false} />
            ))}
          </ControlledCollapse>
        )
      )}
    </>
  );
}

export function ExceptionChain({
  exceptions,
  platform,
}: {
  exceptions: EventException[];
  platform: string;
}): React.JSX.Element {
  const styles = useStyles2(getStyles);
  return (
    <>
      {displayExceptions(exceptions, platform).map((exception, i) => (
        <React.Fragment key={i}>
          {i > 0 && <p className={styles.separator}>{chainSeparator(platform)}</p>}
          <div className={styles.exception}>
            <h4>{exception.type}</h4>
            <p className={styles.value}>{exception.value}</p>
            {exception.handled === false && <Badge color="red" text="Unhandled" />}
            <Frames frames={exception.frames} />
          </div>
        </React.Fragment>
      ))}
    </>
  );
}
