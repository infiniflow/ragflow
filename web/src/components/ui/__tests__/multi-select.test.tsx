import { render, screen } from '@testing-library/react';

import { MultiSelect } from '../multi-select';

// jsdom does not provide ResizeObserver or scrollIntoView, which cmdk
// relies on.
beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  Element.prototype.scrollIntoView = () => {};
});

const noop = () => {};

describe('MultiSelect badge labels', () => {
  it('keeps the label of a selected value after its option disappears', () => {
    const { rerender } = render(
      <MultiSelect
        options={[
          { label: 'Alpha', value: 'a' },
          { label: 'Beta', value: 'b' },
        ]}
        defaultValue={['a']}
        onValueChange={noop}
      />,
    );
    expect(screen.getByText('Alpha')).toBeTruthy();

    // The option list is narrowed (e.g. by a server-side search) and no
    // longer contains the selected value; the badge must not fall back to
    // the raw value.
    rerender(
      <MultiSelect
        options={[{ label: 'Beta', value: 'b' }]}
        defaultValue={['a']}
        onValueChange={noop}
      />,
    );
    expect(screen.getByText('Alpha')).toBeTruthy();
    expect(screen.queryByText('a')).toBeNull();
  });

  it('resolves a never-seen selected value via getOptionLabel', () => {
    render(
      <MultiSelect
        options={[{ label: 'Beta', value: 'b' }]}
        defaultValue={['x']}
        onValueChange={noop}
        getOptionLabel={(value) => (value === 'x' ? 'X-ray' : undefined)}
      />,
    );
    expect(screen.getByText('X-ray')).toBeTruthy();
  });

  it('falls back to the raw value when no label is known', () => {
    render(
      <MultiSelect options={[]} defaultValue={['z']} onValueChange={noop} />,
    );
    expect(screen.getByText('z')).toBeTruthy();
  });
});
