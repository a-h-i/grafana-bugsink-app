// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { render, screen } from '@testing-library/react';
import { IssueDetail } from '../../datasource/types';
import { IssueSummary } from './IssueSummary';

const ISSUE: IssueDetail = {
  id: 'uuid-1',
  friendlyId: 'SAMPLE-PYTHON-2',
  projectId: 1,
  project: 'sample-python',
  type: 'LookupError',
  message: 'failure outside any span',
  transaction: '/no-trace',
  status: 'open',
  firstSeen: '2026-11-13T23:06:27Z',
  lastSeen: '2026-11-15T05:45:33Z',
  events: 26,
  storedEvents: 26,
  isResolved: false,
  isResolvedByNextRelease: false,
  isMuted: false,
  unmuteAfter: null,
  bugsinkUrl: 'http://localhost:8800/issues/issue/uuid-1/event/last/',
};

const value = (label: string) => screen.getByRole('row', { name: new RegExp(`^${label} `) });

describe('IssueSummary', () => {
  it('shows the heading and the rows', () => {
    render(<IssueSummary issue={ISSUE} timeZone="utc" />);

    expect(screen.getByRole('heading', { name: 'LookupError: failure outside any span' })).toBeInTheDocument();
    expect(value('Status')).toHaveTextContent('Open');
    expect(value('Project')).toHaveTextContent('sample-python');
    expect(value('Transaction')).toHaveTextContent('/no-trace');
    expect(value('Events')).toHaveTextContent('26');
    expect(value('Stored events')).toHaveTextContent('26');
    expect(value('First seen')).toHaveTextContent('2026-11-13 23:06:27');
    expect(value('Last seen')).toHaveTextContent('2026-11-15 05:45:33');
  });

  it('labels a muted and a resolved issue', () => {
    const { rerender } = render(<IssueSummary issue={{ ...ISSUE, status: 'muted' }} timeZone="utc" />);
    expect(value('Status')).toHaveTextContent('Muted');

    rerender(<IssueSummary issue={{ ...ISSUE, status: 'resolved' }} timeZone="utc" />);
    expect(value('Status')).toHaveTextContent('Resolved');
  });

  it('shows Unmute after only with a value', () => {
    const { rerender } = render(<IssueSummary issue={ISSUE} timeZone="utc" />);
    expect(screen.queryByText('Unmute after')).not.toBeInTheDocument();

    rerender(<IssueSummary issue={{ ...ISSUE, unmuteAfter: '2026-11-20T10:00:00Z' }} timeZone="utc" />);
    expect(value('Unmute after')).toHaveTextContent('2026-11-20 10:00:00');
  });

  it('shows Resolved by next release only when true', () => {
    const { rerender } = render(<IssueSummary issue={ISSUE} timeZone="utc" />);
    expect(screen.queryByText('Resolved by next release')).not.toBeInTheDocument();

    rerender(<IssueSummary issue={{ ...ISSUE, isResolvedByNextRelease: true }} timeZone="utc" />);
    expect(value('Resolved by next release')).toHaveTextContent('Yes');
  });

  it('shows the type alone when the message is empty', () => {
    render(<IssueSummary issue={{ ...ISSUE, message: '' }} timeZone="utc" />);

    expect(screen.getByRole('heading', { name: 'LookupError' })).toBeInTheDocument();
  });
});
