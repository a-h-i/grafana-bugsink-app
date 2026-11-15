// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { render, screen } from '@testing-library/react';
import { InteractiveTable } from '@grafana/ui';

// Grafana loads its table lazily; once loaded, later renders in the same test file are synchronous.
export async function loadTable(): Promise<void> {
  const { unmount } = render(
    <InteractiveTable columns={[{ id: 'a', header: 'a' }]} data={[{ a: '' }]} getRowId={(row) => row.a} />
  );
  await screen.findByRole('table');
  unmount();
}
