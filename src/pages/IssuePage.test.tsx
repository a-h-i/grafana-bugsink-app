// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { EventRow, IssueDetail } from '../datasource/types';
import { IssuePage } from './IssuePage';

const mockGetList = jest.fn();
const mockGet = jest.fn();
type MockSource = {
  getIssue: jest.Mock;
  getIssueTags: jest.Mock;
  getIssueHistory: jest.Mock;
  listEvents: jest.Mock;
};
const mockSources: Record<string, MockSource> = {};

jest.mock('@grafana/runtime', () => ({
  config: { bootData: { user: { timezone: 'utc' } } },
  getDataSourceSrv: () => ({
    getList: () => mockGetList(),
    get: (uid: string) => mockGet(uid),
  }),
  PluginPage: ({
    actions,
    pageNav,
    children,
  }: {
    actions?: React.ReactNode;
    pageNav?: { text: string };
    children: React.ReactNode;
  }) => (
    <div>
      <h1>{pageNav?.text}</h1>
      <div>{actions}</div>
      {children}
    </div>
  ),
}));

const ISSUE: IssueDetail = {
  id: 'uuid-1',
  friendlyId: 'X-1',
  projectId: 1,
  project: 'sample-python',
  type: 'LookupError',
  message: 'failure',
  transaction: '/x',
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

const TAGS = { tags: [{ key: 'environment', values: [{ value: 'development', count: 26 }], otherValues: 0 }], truncated: false };
const HISTORY = {
  history: [{ id: 1, kind: 'manual_annotation', timestamp: '2026-11-15T05:45:33Z', user: null, comment: 'm4 plan probe' }],
  truncated: false,
};
const EVENT: EventRow = {
  id: 'e1',
  eventId: 'aaaaaaaa-0000-0000-0000-000000000001',
  issue: 'X-1',
  issueId: 'uuid-1',
  project: 'sample-python',
  projectId: 1,
  timestamp: Date.UTC(2026, 10, 15, 5, 45, 33),
  ingestedAt: Date.UTC(2026, 10, 15, 5, 45, 40),
  url: 'http://localhost:8800/issues/issue/uuid-1/event/e1/',
};
const PENDING = () => new Promise(() => {});

function newSource(): MockSource {
  return {
    getIssue: jest.fn().mockResolvedValue(ISSUE),
    getIssueTags: jest.fn().mockResolvedValue(TAGS),
    getIssueHistory: jest.fn().mockResolvedValue(HISTORY),
    listEvents: jest.fn().mockResolvedValue({ rows: [EVENT], notices: [] }),
  };
}

function Location() {
  const { pathname, search } = useLocation();
  return <div data-testid="location">{pathname + search}</div>;
}

function renderPage(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]} future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <Routes>
        <Route path="/a/ahmedismail-bugsink-app/issues/:issueId" element={<IssuePage />} />
        <Route path="*" element={<div>list</div>} />
      </Routes>
      <Location />
    </MemoryRouter>
  );
}

describe('IssuePage', () => {
  beforeEach(() => {
    mockGetList.mockReset().mockReturnValue([
      { uid: 'bugsink', name: 'Bugsink' },
      { uid: 'eu', name: 'Bugsink EU' },
    ]);
    mockGet.mockReset().mockImplementation((uid: string) => Promise.resolve(mockSources[uid]));
    for (const uid of ['bugsink', 'eu']) {
      mockSources[uid] = newSource();
    }
  });

  it('reads the issue on the data source of the address and shows its summary', async () => {
    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?orgId=1&kiosk&ds=eu&state=open&q=a%20b');

    expect(await screen.findByRole('heading', { name: 'LookupError: failure' })).toBeInTheDocument();
    expect(mockSources.eu.getIssue).toHaveBeenCalledWith('X-1');
    expect(mockSources.bugsink.getIssue).not.toHaveBeenCalled();
    expect(screen.getByRole('heading', { name: 'X-1' })).toBeInTheDocument();
  });

  it('goes back to the list with the address parameters exactly as written', async () => {
    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?orgId=1&kiosk&ds=eu&state=open&q=a%20b');
    await screen.findByText('LookupError: failure');

    fireEvent.click(screen.getByRole('button', { name: 'Back to issues' }));

    expect(screen.getByTestId('location')).toHaveTextContent(
      '/a/ahmedismail-bugsink-app?orgId=1&kiosk&ds=eu&state=open&q=a%20b'
    );
  });

  it('shows a placeholder while the issue is pending', async () => {
    mockSources.bugsink.getIssue.mockReturnValue(new Promise(() => {}));

    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink');

    expect(await screen.findByText('Loading issue...')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Open in Bugsink' })).not.toBeInTheDocument();
  });

  it('shows the message of a failed read', async () => {
    mockSources.bugsink.getIssue.mockRejectedValue({
      data: { message: 'Bugsink refused the request: No Issue matches the given query.' },
    });

    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink');

    expect(
      await screen.findByText('Bugsink refused the request: No Issue matches the given query.')
    ).toBeInTheDocument();
  });

  it('links to the issue in Bugsink in a new tab', async () => {
    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink');

    const link = await screen.findByRole('link', { name: 'Open in Bugsink' });
    expect(link).toHaveAttribute('href', ISSUE.bugsinkUrl);
    expect(link).toHaveAttribute('target', '_blank');
  });

  it('refresh reads the issue again', async () => {
    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink');
    await screen.findByText('LookupError: failure');

    await act(async () => fireEvent.click(screen.getByTestId('data-testid RefreshPicker run button')));

    await waitFor(() => expect(mockSources.bugsink.getIssue).toHaveBeenCalledTimes(2));
  });

  it('reads tags, history and events of the issue on the chosen instance', async () => {
    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=eu');

    expect(await screen.findByText('development (26)')).toBeInTheDocument();
    expect(screen.getByText('m4 plan probe')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: EVENT.eventId })).toBeInTheDocument();
    expect(mockSources.eu.getIssueTags).toHaveBeenCalledWith('X-1');
    expect(mockSources.eu.getIssueHistory).toHaveBeenCalledWith('X-1');
    expect(mockSources.eu.listEvents).toHaveBeenCalledWith({ queryType: 'events', issue: 'X-1' });
    expect(mockSources.bugsink.getIssueTags).not.toHaveBeenCalled();
  });

  it('shows the loading text of each section while it is pending', async () => {
    Object.assign(mockSources.bugsink, {
      getIssue: jest.fn(PENDING),
      getIssueTags: jest.fn(PENDING),
      getIssueHistory: jest.fn(PENDING),
      listEvents: jest.fn(PENDING),
    });

    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink');

    for (const text of ['Loading issue...', 'Loading tags...', 'Loading history...', 'Loading events...']) {
      expect(await screen.findByText(text)).toBeInTheDocument();
    }
  });

  it('shows a failed section under its own heading while the others render', async () => {
    const message = 'The API token lacks events:read.';
    mockSources.bugsink.getIssueTags.mockRejectedValue({ data: { message } });
    mockSources.bugsink.listEvents.mockRejectedValue(new Error(message));

    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink');

    expect(await screen.findAllByText(message)).toHaveLength(2);
    for (const name of ['Tags', 'Events']) {
      const section = screen.getByRole('heading', { name }).closest('section')!;
      expect(within(section).getByText(message)).toBeInTheDocument();
    }
    expect(screen.getByRole('heading', { name: 'LookupError: failure' })).toBeInTheDocument();
    expect(screen.getByText('m4 plan probe')).toBeInTheDocument();
    expect(screen.queryByText('Loading tags...')).not.toBeInTheDocument();
  });

  it('shows the message of a failed summary once and no other section', async () => {
    const message = 'Bugsink refused the request: No Issue matches the given query.';
    for (const read of ['getIssue', 'getIssueTags', 'getIssueHistory', 'listEvents'] as const) {
      mockSources.bugsink[read].mockRejectedValue({ data: { message } });
    }

    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink');

    expect(await screen.findAllByText(message)).toHaveLength(1);
    for (const name of ['Tags', 'History', 'Events']) {
      expect(screen.queryByRole('heading', { name })).not.toBeInTheDocument();
    }
    expect(screen.queryByText(/^Loading /)).not.toBeInTheDocument();
  });

  it('keeps the summary of the issue in the address when a late answer arrives', async () => {
    let resolveFirst: (issue: IssueDetail) => void = () => {};
    mockSources.bugsink.getIssue.mockImplementation((id: string) =>
      id === 'X-1'
        ? new Promise<IssueDetail>((resolve) => (resolveFirst = resolve))
        : Promise.resolve({ ...ISSUE, friendlyId: 'X-2', message: 'second' })
    );
    function Jump() {
      const navigate = useNavigate();
      return <button onClick={() => navigate('/a/ahmedismail-bugsink-app/issues/X-2?ds=bugsink')}>jump</button>;
    }
    render(
      <MemoryRouter
        initialEntries={['/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink']}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/a/ahmedismail-bugsink-app/issues/:issueId" element={<IssuePage />} />
        </Routes>
        <Jump />
      </MemoryRouter>
    );
    await screen.findByText('Loading issue...');

    fireEvent.click(screen.getByText('jump'));
    expect(await screen.findByText('LookupError: second')).toBeInTheDocument();
    await act(async () => resolveFirst({ ...ISSUE, message: 'first' }));

    expect(screen.getByText('LookupError: second')).toBeInTheDocument();
    expect(screen.queryByText('LookupError: first')).not.toBeInTheDocument();
  });

  it('shows the notices of the events above the table', async () => {
    mockSources.bugsink.listEvents.mockResolvedValue({
      rows: [EVENT],
      notices: ['Only the newest 1000 events were read.'],
    });

    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink');

    expect(await screen.findByText('Only the newest 1000 events were read.')).toBeInTheDocument();
  });

  it('refresh reads all four sections again', async () => {
    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink');
    await screen.findByText('m4 plan probe');

    await act(async () => fireEvent.click(screen.getByTestId('data-testid RefreshPicker run button')));

    await waitFor(() => {
      for (const read of ['getIssue', 'getIssueTags', 'getIssueHistory', 'listEvents'] as const) {
        expect(mockSources.bugsink[read]).toHaveBeenCalledTimes(2);
      }
    });
  });

  it('explains an unknown data source and reads nothing', async () => {
    renderPage('/a/ahmedismail-bugsink-app/issues/X-1?ds=nosuch');

    expect(await screen.findByText('The data source "nosuch" was not found.')).toBeInTheDocument();
    expect(mockSources.bugsink.getIssue).not.toHaveBeenCalled();
    expect(mockSources.eu.getIssue).not.toHaveBeenCalled();
  });

  it('explains a missing data source', () => {
    mockGetList.mockReturnValue([]);

    renderPage('/a/ahmedismail-bugsink-app/issues/X-1');

    expect(screen.getByText('No Bugsink data source yet.')).toBeInTheDocument();
  });
});
