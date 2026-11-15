// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { EventException, StackFrame } from '../../datasource/types';
import { ExceptionChain } from './StackTrace';

function frame(overrides: Partial<StackFrame> = {}): StackFrame {
  return {
    filename: 'lib.py',
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

function exception(type: string, overrides: Partial<EventException> = {}): EventException {
  return {
    type,
    value: `${type} value`,
    module: '',
    mechanism: '',
    handled: true,
    frames: [],
    framesOmitted: 0,
    ...overrides,
  };
}

describe('ExceptionChain', () => {
  it('shows a Python chain in payload order with the separator between', () => {
    const { container } = render(
      <ExceptionChain exceptions={[exception('JSONDecodeError'), exception('RuntimeError')]} platform="python" />
    );

    const headings = screen.getAllByRole('heading', { level: 4 }).map((h) => h.textContent);
    expect(headings).toEqual(['JSONDecodeError', 'RuntimeError']);
    expect(screen.getByText('During handling of the above exception, another exception occurred:')).toBeInTheDocument();
    expect(container.textContent?.indexOf('During handling')).toBeGreaterThan(
      container.textContent!.indexOf('JSONDecodeError value')
    );
    expect(container.textContent?.indexOf('During handling')).toBeLessThan(
      container.textContent!.indexOf('RuntimeError value')
    );
  });

  it('names an exception without a type', () => {
    render(<ExceptionChain exceptions={[exception('', { value: 'boom' })]} platform="python" />);

    expect(screen.getByRole('heading', { level: 4, name: 'Unknown exception' })).toBeInTheDocument();
    expect(screen.getByText('boom')).toBeInTheDocument();
  });

  it('shows the value of an exception as text', () => {
    render(
      <ExceptionChain exceptions={[exception('Boom', { value: '<img src=x onerror=alert(1)>' })]} platform="python" />
    );

    expect(screen.getByText('<img src=x onerror=alert(1)>')).toBeInTheDocument();
    expect(document.querySelector('img')).toBeNull();
  });

  it('marks only unhandled exceptions', () => {
    const { rerender } = render(<ExceptionChain exceptions={[exception('A', { handled: false })]} platform="python" />);
    expect(screen.getByText('Unhandled')).toBeInTheDocument();

    rerender(
      <ExceptionChain
        exceptions={[exception('A', { handled: true }), exception('B', { handled: null })]}
        platform="python"
      />
    );
    expect(screen.queryByText('Unhandled')).not.toBeInTheDocument();
  });

  it('shows an in-app frame with its header and numbered source around the failing line', () => {
    const app = frame({
      filename: 'app.py',
      function: '_fail',
      lineno: 60,
      inApp: true,
      preContext: ['def _fail():', '    data = {}'],
      contextLine: '    raise RuntimeError("x")',
      postContext: ['', 'def other():'],
    });
    const { container } = render(<ExceptionChain exceptions={[exception('A', { frames: [app] })]} platform="python" />);

    expect(screen.getByText('app.py:60 in _fail')).toBeInTheDocument();
    const pre = container.querySelector('pre');
    expect(pre?.textContent).toContain('58 | def _fail():');
    expect(pre?.textContent).toContain('59 |     data = {}');
    expect(pre?.textContent).toContain('61 | ');
    expect(pre?.textContent).toContain('62 | def other():');
    expect(pre?.querySelector('mark')?.textContent).toBe('60 |     raise RuntimeError("x")');
  });

  it('shows source lines as text', () => {
    const app = frame({ inApp: true, lineno: 3, contextLine: '<script>x</script>' });
    const { container } = render(<ExceptionChain exceptions={[exception('A', { frames: [app] })]} platform="python" />);

    expect(container.querySelector('mark')?.textContent).toBe('3 | <script>x</script>');
    expect(container.querySelector('script')).toBeNull();
  });

  it('shows every frame with its source and no fold when none is in the app', () => {
    const lib = (name: string) => frame({ filename: `${name}.py`, lineno: 3, contextLine: `call_${name}()` });
    const { container } = render(
      <ExceptionChain exceptions={[exception('A', { frames: [lib('a'), lib('b')] })]} platform="python" />
    );

    expect(container.querySelectorAll('pre')).toHaveLength(2);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('folds frames outside the app behind a button until it is clicked', () => {
    const frames = [
      frame({ filename: 'starlette.py' }),
      frame({ filename: 'anyio.py' }),
      frame({ filename: 'app.py', inApp: true }),
    ];
    render(<ExceptionChain exceptions={[exception('A', { frames })]} platform="python" />);

    expect(screen.getByText('app.py')).toBeInTheDocument();
    expect(screen.queryByText('starlette.py')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '2 other frames' }));

    expect(screen.getByText('starlette.py')).toBeInTheDocument();
    expect(screen.getByText('anyio.py')).toBeInTheDocument();
  });

  it('uses the singular for one folded frame', () => {
    const frames = [frame({ filename: 'a.py', inApp: true }), frame({ filename: 'b.py' })];
    render(<ExceptionChain exceptions={[exception('A', { frames })]} platform="python" />);

    expect(screen.getByRole('button', { name: '1 other frame' })).toBeInTheDocument();
  });

  it('says when an exception has no frames', () => {
    render(<ExceptionChain exceptions={[exception('A')]} platform="python" />);

    expect(screen.getByText('No frames.')).toBeInTheDocument();
  });
});
