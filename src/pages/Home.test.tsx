// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { render, screen } from '@testing-library/react';
import { locationUtil } from '@grafana/data';
import { MemoryRouter } from 'react-router-dom';
import { Home } from './Home';

const getList = jest.fn();

jest.mock('@grafana/runtime', () => ({
  getDataSourceSrv: () => ({ getList }),
  PluginPage: ({ children, pageNav }: { children: React.ReactNode; pageNav?: { text: string } }) => (
    <div>
      {pageNav && <h2>{pageNav.text}</h2>}
      {children}
    </div>
  ),
}));

describe('Home', () => {
  beforeEach(() => getList.mockReset());

  it('lists Bugsink data sources', () => {
    getList.mockReturnValue([
      { uid: 'bugsink', name: 'Bugsink', type: 'ahmedismail-bugsink-datasource' },
      { uid: 'eu', name: 'Bugsink EU', type: 'ahmedismail-bugsink-datasource' },
      { uid: 'grafana', name: '-- Grafana --', type: 'grafana' },
    ]);

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    expect(getList).toHaveBeenCalledWith({ type: 'ahmedismail-bugsink-datasource' });
    expect(screen.queryByRole('heading')).not.toBeInTheDocument();
    expect(
      screen.getByText(
        'Issue browsing arrives in a later version. For now, connect Grafana to Bugsink with a data source.'
      )
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Bugsink' }).getAttribute('href')).toMatch(
      /\/connections\/datasources\/edit\/bugsink$/
    );
    expect(screen.queryByRole('link', { name: '-- Grafana --' })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Bugsink EU' }).getAttribute('href')).toMatch(
      /\/connections\/datasources\/edit\/eu$/
    );
  });

  it('explains what to do when there are none', () => {
    getList.mockReturnValue([]);

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    expect(screen.getByText('No Bugsink data source yet.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Add a Bugsink data source' }).getAttribute('href')).toMatch(
      /\/connections\/datasources\/new$/
    );
  });

  it('prefixes the add link with the Grafana sub-path', () => {
    getList.mockReturnValue([]);
    const init = (appSubUrl: string) =>
      locationUtil.initialize({
        config: { appSubUrl } as never,
        getVariablesUrlParams: jest.fn(),
        getTimeRangeForUrl: jest.fn(),
      });
    init('/grafana');

    try {
      render(
        <MemoryRouter basename="/grafana" initialEntries={['/grafana']}>
          <Home />
        </MemoryRouter>
      );

      expect(screen.getByRole('link', { name: 'Add a Bugsink data source' })).toHaveAttribute(
        'href',
        '/grafana/connections/datasources/new'
      );
    } finally {
      init('');
    }
  });
});
