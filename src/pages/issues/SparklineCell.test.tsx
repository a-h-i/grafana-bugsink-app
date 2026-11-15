// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { act, render, screen } from '@testing-library/react';
import { VolumeSeries } from '../../datasource/types';
import { IssueTableContext, VolumeSource } from './IssueTableContext';
import { SparklineCell } from './SparklineCell';

const times = [1000, 2000, 3000];

const pending = () => jest.fn((_id: string, _signal: AbortSignal) => new Promise<VolumeSeries>(() => {}));

function renderCell(source: VolumeSource, id = 'a1') {
  const ui = (s: VolumeSource, i: string) => (
    <IssueTableContext.Provider value={{ source: s, issueHref: () => '/' }}>
      <SparklineCell id={i} />
    </IssueTableContext.Provider>
  );
  const view = render(ui(source, id));
  return { ...view, again: (s: VolumeSource, i = id) => view.rerender(ui(s, i)) };
}

function deferred() {
  let resolve: (value: VolumeSeries) => void = () => {};
  let reject: (reason: unknown) => void = () => {};
  const promise = new Promise<VolumeSeries>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const BOX = { width: '120px', height: '24px' };

describe('SparklineCell', () => {
  it('throws outside an issue table', () => {
    const spy = jest.spyOn(console, 'error').mockImplementation(() => {});

    expect(() => render(<SparklineCell id="a1" />)).toThrow('IssueTableContext is missing');
    spy.mockRestore();
  });

  it('keeps one 120 by 24 box in every state', async () => {
    const loading = renderCell({ issueVolume: pending() });
    expect(screen.getByRole('img', { name: 'Loading the last 24 hours' })).toHaveStyle(BOX);
    loading.unmount();

    const line = renderCell({ issueVolume: jest.fn().mockResolvedValue({ times, counts: [0, 3, 4], total: 7 }) });
    expect(await screen.findByRole('img', { name: '7 events in the last 24 hours' })).toHaveStyle(BOX);
    line.unmount();

    const none = renderCell({ issueVolume: jest.fn().mockResolvedValue({ times, counts: [0, 0, 0], total: 0 }) });
    expect(await screen.findByRole('img', { name: 'No events in the last 24 hours' })).toHaveStyle(BOX);
    none.unmount();

    renderCell({ issueVolume: jest.fn().mockRejectedValue(new Error('boom')) });
    expect(await screen.findByRole('img', { name: 'Events per hour unavailable: boom' })).toHaveStyle(BOX);
  });

  it('lets the keyboard reach the failure', async () => {
    renderCell({ issueVolume: jest.fn().mockRejectedValue(new Error('boom')) });

    expect(await screen.findByRole('img', { name: 'Events per hour unavailable: boom' })).toHaveAttribute(
      'tabindex',
      '0'
    );
  });

  it('shows the data source message, or a default one, for a failure', async () => {
    renderCell({ issueVolume: jest.fn().mockRejectedValue({}) });

    expect(
      await screen.findByRole('img', { name: 'Events per hour unavailable: The event volume could not be loaded.' })
    ).toBeInTheDocument();
  });

  it('does not rebuild the chart when the context changes', async () => {
    const source = { issueVolume: jest.fn().mockResolvedValue({ times, counts: [0, 3, 4], total: 7 }) };
    const ui = (busy: boolean) => (
      <IssueTableContext.Provider value={{ source, issueHref: () => '/', disabled: busy }}>
        <SparklineCell id="a1" />
      </IssueTableContext.Provider>
    );
    const view = render(ui(false));
    const chart = (await screen.findByTestId('uplot-main-div')).firstElementChild;
    expect(chart).not.toBeNull();

    view.rerender(ui(true));

    expect(screen.getByTestId('uplot-main-div').firstElementChild).toBe(chart);
  });

  it('keeps the chart out of the tab order and gives it the total as a title', async () => {
    renderCell({ issueVolume: jest.fn().mockResolvedValue({ times, counts: [0, 3, 4], total: 7 }) });

    const chart = await screen.findByRole('img', { name: '7 events in the last 24 hours' });

    expect(chart).not.toHaveAttribute('tabindex');
    expect(chart).toHaveAttribute('title', '7 events in the last 24 hours');
  });

  it('shows a loading state while the request is pending', () => {
    renderCell({ issueVolume: pending() });

    expect(screen.getByRole('img', { name: 'Loading the last 24 hours' })).toBeInTheDocument();
  });

  it('draws the line with the total as its text alternative', async () => {
    renderCell({
      issueVolume: jest.fn().mockResolvedValue({ times, counts: [0, 3, 4], total: 7 }),
    });

    expect(await screen.findByLabelText('7 events in the last 24 hours')).toBeInTheDocument();
    expect(screen.getByTestId('uplot-main-div')).toBeInTheDocument();
  });

  it('says No events when every hour is zero', async () => {
    renderCell({ issueVolume: jest.fn().mockResolvedValue({ times, counts: [0, 0, 0], total: 0 }) });

    expect(await screen.findByText('No events')).toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'No events in the last 24 hours' })).toBeInTheDocument();
  });

  it('shows the failure as an icon with the message', async () => {
    const message = 'Bugsink refused the request: No Issue matches the given query.';
    renderCell({ issueVolume: jest.fn().mockRejectedValue(new Error(message)) });

    expect(await screen.findByLabelText(`Events per hour unavailable: ${message}`)).toBeInTheDocument();
  });

  it('aborts the old request and asks again for a new source', () => {
    const first = { issueVolume: pending() };
    const second = { issueVolume: pending() };
    const { again } = renderCell(first);
    const signal: AbortSignal = first.issueVolume.mock.calls[0][1];

    again(second);

    expect(signal.aborted).toBe(true);
    expect(second.issueVolume).toHaveBeenCalledWith('a1', expect.any(AbortSignal));
    expect(second.issueVolume.mock.calls[0][1].aborted).toBe(false);
  });

  it('shows loading again for a new id and ignores the old answer', async () => {
    const old = deferred();
    const source = {
      issueVolume: jest.fn((id: string) => (id === 'a1' ? old.promise : new Promise<VolumeSeries>(() => {}))),
    };
    const { again } = renderCell(source);

    again(source, 'a2');
    await act(async () => old.resolve({ times, counts: [1, 1, 1], total: 3 }));

    expect(screen.getByLabelText('Loading the last 24 hours')).toBeInTheDocument();
    expect(screen.queryByLabelText('3 events in the last 24 hours')).not.toBeInTheDocument();
  });

  it('aborts on unmount and logs nothing when the aborted request rejects', async () => {
    const spy = jest.spyOn(console, 'error').mockImplementation(() => {});
    const answer = deferred();
    const source = { issueVolume: jest.fn((_id: string, _signal: AbortSignal) => answer.promise) };
    const { unmount } = renderCell(source);
    const signal: AbortSignal = source.issueVolume.mock.calls[0][1];

    unmount();
    await act(async () => answer.reject(Object.assign(new Error('aborted'), { name: 'AbortError' })));

    expect(signal.aborted).toBe(true);
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });
});
