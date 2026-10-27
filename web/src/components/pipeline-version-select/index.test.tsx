import { render, screen } from '@testing-library/react';
import React from 'react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { PipelineVersionSelect, filterVersionsByAlias } from './index';

describe('filterVersionsByAlias', () => {
  const versions = [
    { id: 'v1', title: 'First', alias: 'prod' },
    { id: 'v2', title: 'Second', alias: null },
    { id: 'v3', title: 'Third', alias: 'PROD-2' },
  ];

  it('returns all versions when the filter is empty or whitespace', () => {
    expect(filterVersionsByAlias(versions, '')).toHaveLength(3);
    expect(filterVersionsByAlias(versions, '   ')).toHaveLength(3);
  });

  it('filters case-insensitively by alias', () => {
    expect(filterVersionsByAlias(versions, 'prod').map((v) => v.id)).toEqual([
      'v1',
      'v3',
    ]);
  });

  it('treats a null alias as non-matching', () => {
    expect(filterVersionsByAlias(versions, 'second')).toHaveLength(0);
  });
});

describe('PipelineVersionSelect (G1/v5.2: no latest pseudo-option)', () => {
  it('renders the alias filter input', () => {
    render(
      React.createElement(PipelineVersionSelect, {
        versions: [],
        value: '',
        onChange: jest.fn(),
      }),
    );
    expect(
      screen.getByPlaceholderText('flow.version.filterByAlias'),
    ).toBeInTheDocument();
  });

  it('shows the latest placeholder when unbound, with no __latest__ option', () => {
    render(
      React.createElement(PipelineVersionSelect, {
        versions: [{ id: 'v1', title: 'First', alias: null }],
        value: '',
        onChange: jest.fn(),
      }),
    );
    // The "latest" label is only a placeholder; there is no selectable
    // __latest__ sentinel item (design G1 / v5.2).
    expect(screen.getByText('flow.version.latest')).toBeInTheDocument();
    expect(screen.queryByText('__latest__')).toBeNull();
  });

  it('passes the concrete version id through unchanged', () => {
    const onChange = jest.fn();
    render(
      React.createElement(PipelineVersionSelect, {
        versions: [{ id: 'v1', title: 'First', alias: 'prod' }],
        value: 'v1',
        onChange,
      }),
    );
    expect(screen.queryByText('__latest__')).toBeNull();
  });
});
