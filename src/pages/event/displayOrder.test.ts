// SPDX-License-Identifier: GPL-3.0-or-later
import { EventException, StackFrame } from '../../datasource/types';
import { chainSeparator, displayExceptions, frameGroups, frameTitle } from './displayOrder';

function frame(overrides: Partial<StackFrame> = {}): StackFrame {
  return {
    filename: 'app.py',
    function: '',
    module: '',
    lineno: null,
    colno: null,
    inApp: false,
    preContext: [],
    contextLine: null,
    postContext: [],
    ...overrides,
  };
}

function exception(type: string, frames: StackFrame[]): EventException {
  return { type, value: '', module: '', mechanism: '', handled: true, frames, framesOmitted: 0 };
}

const first = exception('First', [frame({ filename: 'a' }), frame({ filename: 'b' })]);
const second = exception('Second', [frame({ filename: 'c' }), frame({ filename: 'd' })]);

describe('displayExceptions', () => {
  it('keeps the order of a Python event', () => {
    const shown = displayExceptions([first, second], 'python');

    expect(shown.map((e) => e.type)).toEqual(['First', 'Second']);
    expect(shown[0].frames.map((f) => f.filename)).toEqual(['a', 'b']);
  });

  it.each(['javascript', ''])(
    'reverses the exceptions and frames of a %j event without changing the input',
    (platform) => {
      const shown = displayExceptions([first, second], platform);

      expect(shown.map((e) => e.type)).toEqual(['Second', 'First']);
      expect(shown[0].frames.map((f) => f.filename)).toEqual(['d', 'c']);
      expect(first.frames.map((f) => f.filename)).toEqual(['a', 'b']);
    }
  );
});

describe('chainSeparator', () => {
  it('names the Python wording for Python and the cause wording for other platforms', () => {
    expect(chainSeparator('python')).toBe('During handling of the above exception, another exception occurred:');
    expect(chainSeparator('javascript')).toBe('Caused by:');
    expect(chainSeparator('')).toBe('Caused by:');
  });
});

describe('frameGroups', () => {
  it('folds consecutive frames that are not in the app', () => {
    const ext1 = frame({ filename: 'x1' });
    const ext2 = frame({ filename: 'x2' });
    const app = frame({ filename: 'app', inApp: true });
    const ext3 = frame({ filename: 'x3' });

    expect(frameGroups([ext1, ext2, app, ext3])).toEqual([
      { kind: 'folded', frames: [ext1, ext2] },
      { kind: 'frame', frame: app },
      { kind: 'folded', frames: [ext3] },
    ]);
  });

  it('shows every frame when none is in the app', () => {
    const frames = [frame({ filename: 'x1' }), frame({ filename: 'x2' })];

    expect(frameGroups(frames)).toEqual(frames.map((f) => ({ kind: 'frame', frame: f })));
  });
});

describe('frameTitle', () => {
  it.each([
    [frame({ filename: 'app.py', lineno: 60, function: '_fail' }), 'app.py:60 in _fail'],
    [frame({ filename: 'index.js', lineno: 10, colno: 5, function: 'onClick' }), 'index.js:10:5 in onClick'],
    [frame({ filename: 'app.py' }), 'app.py'],
    [frame({ filename: 'a.js', lineno: null, colno: 5, function: 'f' }), 'a.js in f'],
  ])('is built from what the frame has', (f, title) => {
    expect(frameTitle(f)).toBe(title);
  });
});
