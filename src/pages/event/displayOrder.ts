// SPDX-License-Identifier: GPL-3.0-or-later
import { EventException, StackFrame } from '../../datasource/types';

export type FrameGroup = { kind: 'frame'; frame: StackFrame } | { kind: 'folded'; frames: StackFrame[] };

export function displayExceptions(exceptions: EventException[], platform: string): EventException[] {
  if (platform === 'python') {
    return exceptions;
  }
  return [...exceptions].reverse().map((exception) => ({ ...exception, frames: [...exception.frames].reverse() }));
}

export function chainSeparator(platform: string): string {
  return platform === 'python' ? 'During handling of the above exception, another exception occurred:' : 'Caused by:';
}

export function frameGroups(frames: StackFrame[]): FrameGroup[] {
  if (!frames.some((frame) => frame.inApp)) {
    return frames.map((frame) => ({ kind: 'frame', frame }));
  }
  const groups: FrameGroup[] = [];
  for (const frame of frames) {
    const last = groups[groups.length - 1];
    if (frame.inApp) {
      groups.push({ kind: 'frame', frame });
    } else if (last?.kind === 'folded') {
      last.frames.push(frame);
    } else {
      groups.push({ kind: 'folded', frames: [frame] });
    }
  }
  return groups;
}

export function frameTitle(frame: StackFrame): string {
  const place = [frame.filename, frame.lineno, frame.lineno === null ? null : frame.colno]
    .filter((part) => part !== null && part !== '')
    .join(':');
  if (!frame.function) {
    return place;
  }
  return place ? `${place} in ${frame.function}` : `in ${frame.function}`;
}
