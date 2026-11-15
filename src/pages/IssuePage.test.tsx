// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { EventDetail, EventRow, IssueDetail } from '../datasource/types';
import { IssuePage } from './IssuePage';

const mockGetList = jest.fn();
const mockGet = jest.fn();
type MockSource = {
  getIssue: jest.Mock;
  getIssueTags: jest.Mock;
  getIssueHistory: jest.Mock;
  listEvents: jest.Mock;
  getEvent: jest.Mock;
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

const TAGS = {
  tags: [{ key: 'environment', values: [{ value: 'development', count: 26 }], otherValues: 0 }],
  truncated: false,
};
const HISTORY = {
  history: [
    { id: 1, kind: 'manual_annotation', timestamp: '2026-11-15T05:45:33Z', user: null, comment: 'm4 plan probe' },
  ],
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
const EVENT_2: EventRow = { ...EVENT, id: 'e2', eventId: 'aaaaaaaa-0000-0000-0000-000000000002' };
const PENDING = () => new Promise(() => {});

function eventDetail(id: string, overrides: Partial<EventDetail> = {}): EventDetail {
  return {
    id,
    eventId: `sdk-${id}`,
    issueId: 'uuid-1',
    projectId: 1,
    project: 'sample-python',
    timestamp: '2026-11-15T07:52:59.387Z',
    ingestedAt: '2026-11-15T07:53:00Z',
    platform: 'python',
    level: 'error',
    message: '',
    transaction: '',
    environment: '',
    release: '',
    traceId: '',
    tags: [],
    contexts: [],
    exceptions: [],
    breadcrumbs: [],
    stacktraceMd: null,
    notices: [],
    bugsinkUrl: `http://bugsink/${id}`,
    links: [],
    ...overrides,
  };
}

function frame(inApp: boolean, name: string) {
  return {
    filename: `${name}.py`,
    function: name,
    module: '',
    lineno: 1,
    colno: null,
    inApp,
    preContext: [],
    contextLine: null,
    postContext: [],
  };
}

function newSource(): MockSource {
  return {
    getIssue: jest.fn().mockResolvedValue(ISSUE),
    getIssueTags: jest.fn().mockResolvedValue(TAGS),
    getIssueHistory: jest.fn().mockResolvedValue(HISTORY),
    listEvents: jest.fn().mockResolvedValue({ rows: [EVENT], notices: [] }),
    getEvent: jest.fn((id: string) => Promise.resolve(eventDetail(id))),
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
    expect(await screen.findByRole('link', { name: `Open event ${EVENT.eventId} in Bugsink` })).toBeInTheDocument();
    expect(screen.getByText(EVENT.eventId)).toBeInTheDocument();
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
      getEvent: jest.fn(PENDING),
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

  describe('Event section', () => {
    const PATH = '/a/ahmedismail-bugsink-app/issues/X-1';

    function twoEvents() {
      mockSources.bugsink.listEvents.mockResolvedValue({ rows: [EVENT, EVENT_2], notices: [] });
    }

    it('reads the newest event once the list is in, between the summary and the events', async () => {
      twoEvents();

      renderPage(`${PATH}?ds=bugsink`);

      expect(await screen.findByText('sdk-e1')).toBeInTheDocument();
      expect(mockSources.bugsink.getEvent).toHaveBeenCalledTimes(1);
      expect(mockSources.bugsink.getEvent).toHaveBeenCalledWith('e1');
      const summary = screen.getByRole('heading', { name: 'LookupError: failure' });
      const event = screen.getByRole('heading', { name: 'Event' });
      const events = screen.getByRole('heading', { name: 'Events' });
      expect(summary.compareDocumentPosition(event) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect(event.compareDocumentPosition(events) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it('reads the event of the address without waiting for the list', async () => {
      mockSources.bugsink.listEvents.mockReturnValue(PENDING());

      renderPage(`${PATH}?ds=bugsink&event=e2`);

      expect(await screen.findByText('sdk-e2')).toBeInTheDocument();
      expect(mockSources.bugsink.getEvent).toHaveBeenCalledWith('e2');
    });

    it('shows another event when its id is clicked and puts it in the address', async () => {
      twoEvents();
      renderPage(`${PATH}?orgId=1&kiosk&ds=bugsink`);
      await screen.findByText('sdk-e1');

      fireEvent.click(screen.getByRole('link', { name: EVENT_2.eventId }));

      expect(await screen.findByText('sdk-e2')).toBeInTheDocument();
      expect(mockSources.bugsink.getEvent).toHaveBeenCalledWith('e2');
      expect(screen.getByTestId('location')).toHaveTextContent(`${PATH}?orgId=1&kiosk&ds=bugsink&event=e2`);
      expect(screen.queryByText('sdk-e1')).not.toBeInTheDocument();
    });

    it('shows the event clicked last when the first answer arrives late', async () => {
      twoEvents();
      let resolveFirst: (event: EventDetail) => void = () => {};
      mockSources.bugsink.getEvent.mockImplementation((id: string) =>
        id === 'e1' ? new Promise<EventDetail>((resolve) => (resolveFirst = resolve)) : Promise.resolve(eventDetail(id))
      );
      renderPage(`${PATH}?ds=bugsink`);
      fireEvent.click(await screen.findByRole('link', { name: EVENT_2.eventId }));
      expect(await screen.findByText('sdk-e2')).toBeInTheDocument();

      await act(async () => resolveFirst(eventDetail('e1')));

      expect(screen.getByText('sdk-e2')).toBeInTheDocument();
      expect(screen.queryByText('sdk-e1')).not.toBeInTheDocument();
    });

    it('shows a refused event id under Event while the other sections render', async () => {
      mockSources.bugsink.getEvent.mockRejectedValue({ data: { message: '"nosuch" is not an event id.' } });

      renderPage(`${PATH}?ds=bugsink&event=nosuch`);

      const section = (await screen.findByRole('heading', { name: 'Event' })).closest('section')!;
      expect(await within(section).findByText('"nosuch" is not an event id.')).toBeInTheDocument();
      expect(await screen.findByText('development (26)')).toBeInTheDocument();
      expect(screen.getByText('m4 plan probe')).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'Events' })).toBeInTheDocument();
    });

    it('shows that an event of another issue belongs to another issue', async () => {
      mockSources.bugsink.getEvent.mockResolvedValue(eventDetail('e2', { issueId: 'other' }));

      renderPage(`${PATH}?ds=bugsink&event=e2`);

      const section = (await screen.findByRole('heading', { name: 'Event' })).closest('section')!;
      expect(await within(section).findByText('This event belongs to another issue.')).toBeInTheDocument();
      expect(screen.queryByText('sdk-e2')).not.toBeInTheDocument();
      expect(await screen.findByText('development (26)')).toBeInTheDocument();
      expect(screen.getByText('m4 plan probe')).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'Events' })).toBeInTheDocument();
    });

    it('shows a loading text while the event is pending', async () => {
      mockSources.bugsink.getEvent.mockReturnValue(PENDING());

      renderPage(`${PATH}?ds=bugsink`);

      expect(await screen.findByText('Loading event...')).toBeInTheDocument();
    });

    it('keeps the Event section pending until the summary has loaded', async () => {
      let resolveIssue: (issue: IssueDetail) => void = () => {};
      mockSources.bugsink.getIssue.mockReturnValue(new Promise<IssueDetail>((resolve) => (resolveIssue = resolve)));

      renderPage(`${PATH}?ds=bugsink`);

      await waitFor(() => expect(mockSources.bugsink.getEvent).toHaveBeenCalledWith('e1'));
      const section = (await screen.findByRole('heading', { name: 'Event' })).closest('section')!;
      expect(within(section).getByText('Loading event...')).toBeInTheDocument();
      expect(screen.queryByText('sdk-e1')).not.toBeInTheDocument();

      await act(async () => resolveIssue(ISSUE));

      expect(await screen.findByText('sdk-e1')).toBeInTheDocument();
      expect(within(section).queryByText('Loading event...')).not.toBeInTheDocument();
    });

    it('has no Event section when the issue has no events', async () => {
      mockSources.bugsink.listEvents.mockResolvedValue({ rows: [], notices: [] });

      renderPage(`${PATH}?ds=bugsink`);

      expect(await screen.findByText('No stored events.')).toBeInTheDocument();
      expect(screen.queryByRole('heading', { name: 'Event' })).not.toBeInTheDocument();
      expect(mockSources.bugsink.getEvent).not.toHaveBeenCalled();
    });

    it('shows the notices of the event as warnings', async () => {
      mockSources.bugsink.getEvent.mockResolvedValue(
        eventDetail('e1', { notices: ['Only the 250 frames nearest the error are shown for RuntimeError.'] })
      );

      renderPage(`${PATH}?ds=bugsink`);

      const section = (await screen.findByRole('heading', { name: 'Event' })).closest('section')!;
      expect(
        await within(section).findByText('Only the 250 frames nearest the error are shown for RuntimeError.')
      ).toBeInTheDocument();
    });

    it('does not keep a fold open on the next event', async () => {
      twoEvents();
      const exceptions = [
        {
          type: 'RuntimeError',
          value: 'boom',
          module: '',
          mechanism: '',
          handled: true,
          frames: [frame(false, 'lib_a'), frame(false, 'lib_b'), frame(true, 'app')],
          framesOmitted: 0,
        },
      ];
      mockSources.bugsink.getEvent.mockImplementation((id: string) => Promise.resolve(eventDetail(id, { exceptions })));
      renderPage(`${PATH}?ds=bugsink`);
      fireEvent.click(await screen.findByText('2 other frames'));
      expect(screen.getByText(/lib_a/)).toBeInTheDocument();

      fireEvent.click(screen.getByRole('link', { name: EVENT_2.eventId }));

      expect(await screen.findByText('2 other frames')).toBeInTheDocument();
      expect(screen.queryByText(/lib_a/)).not.toBeInTheDocument();
    });

    it('goes back to the list without the event and with the other parameters as written', async () => {
      renderPage(`${PATH}?orgId=1&kiosk&ds=eu&event=e2`);
      await screen.findByText('sdk-e2');

      fireEvent.click(screen.getByRole('button', { name: 'Back to issues' }));

      expect(screen.getByTestId('location')).toHaveTextContent('/a/ahmedismail-bugsink-app?orgId=1&kiosk&ds=eu');
      expect(screen.getByTestId('location')).not.toHaveTextContent('event=');
    });

    it('refresh reads the event again', async () => {
      renderPage(`${PATH}?ds=bugsink`);
      await screen.findByText('sdk-e1');

      await act(async () => fireEvent.click(screen.getByTestId('data-testid RefreshPicker run button')));

      await waitFor(() => expect(mockSources.bugsink.getEvent).toHaveBeenCalledTimes(2));
    });

    it('shows no section for an unknown data source', async () => {
      renderPage(`${PATH}?ds=nosuch`);

      expect(await screen.findByText('The data source "nosuch" was not found.')).toBeInTheDocument();
      for (const name of ['Tags', 'History', 'Events', 'Event']) {
        expect(screen.queryByRole('heading', { name })).not.toBeInTheDocument();
      }
    });
  });
});
