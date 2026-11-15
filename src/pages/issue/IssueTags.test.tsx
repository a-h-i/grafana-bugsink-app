// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { render, screen } from '@testing-library/react';
import { IssueTags as Tags } from '../../datasource/types';
import { IssueTags } from './IssueTags';

const values = [
  { value: 'development', count: 26 },
  { value: 'staging', count: 3 },
];

function renderTags(tags: Tags) {
  return render(<IssueTags tags={tags} />);
}

describe('IssueTags', () => {
  it('lists the values of a key with their counts', () => {
    renderTags({ tags: [{ key: 'environment', values, otherValues: 0 }], truncated: false });

    expect(screen.getByText('environment')).toBeInTheDocument();
    expect(screen.getByText('development (26), staging (3)')).toBeInTheDocument();
  });

  it('says how many other values a key has', () => {
    renderTags({
      tags: [
        { key: 'trace', values, otherValues: 76 },
        { key: 'span', values, otherValues: 1 },
      ],
      truncated: false,
    });

    expect(screen.getByText('development (26), staging (3) and 76 more values')).toBeInTheDocument();
    expect(screen.getByText('development (26), staging (3) and 1 more value')).toBeInTheDocument();
  });

  it('warns when the read was cut short', () => {
    renderTags({ tags: [{ key: 'environment', values, otherValues: 0 }], truncated: true });

    expect(screen.getByText('Only the first 5000 tag values were read.')).toBeInTheDocument();
  });

  it('does not warn when the read was complete', () => {
    renderTags({ tags: [{ key: 'environment', values, otherValues: 0 }], truncated: false });

    expect(screen.queryByText(/Only the first/)).not.toBeInTheDocument();
  });

  it('explains an issue without tags', () => {
    renderTags({ tags: [], truncated: false });

    expect(screen.getByText('No tags.')).toBeInTheDocument();
  });
});
