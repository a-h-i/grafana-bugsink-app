// SPDX-License-Identifier: GPL-3.0-or-later
import { expect, test as setup } from '@grafana/plugin-e2e';
import { basicAuth } from './fixtures';
import { EDITOR, VIEWER } from './seeded';

setup.describe.configure({ retries: 0 });

const ADMIN = basicAuth({ user: 'admin', password: 'admin' });

const users = [
  { account: EDITOR, role: 'Editor' },
  { account: VIEWER, role: 'Viewer' },
];

for (const { account, role } of users) {
  setup(`make ${account.user} an ${role}`, async ({ request }) => {
    const created = await request.post('/api/admin/users', {
      headers: ADMIN,
      data: {
        name: account.user,
        login: account.user,
        email: `${account.user}@example.com`,
        password: account.password,
      },
    });
    let id: number;
    if (created.status() === 412) {
      const found = await request.get('/api/users/lookup', {
        headers: ADMIN,
        params: { loginOrEmail: account.user },
      });
      expect(found.ok()).toBe(true);
      id = (await found.json()).id;
    } else {
      expect(created.ok()).toBe(true);
      id = (await created.json()).id;
    }
    const signedIn = await request.get('/api/user', { headers: basicAuth(account) });
    expect(
      signedIn.ok() && (await signedIn.json()).login,
      `${account.user} exists with another password; delete the user in Grafana and run again`
    ).toBe(account.user);
    const patched = await request.patch(`/api/orgs/1/users/${id}`, { headers: ADMIN, data: { role } });
    expect(patched.ok()).toBe(true);
  });
}
