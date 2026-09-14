import { fireEvent, render, screen } from '@testing-library/react';

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

describe('MultiSelect disabled options', () => {
  const options = [
    { label: 'Alpha', value: 'a', disabled: true },
    { label: 'Beta', value: 'b' },
  ];

  it('removes a disabled selected value via its badge remove icon', () => {
    const onValueChange = jest.fn();
    render(
      <MultiSelect
        options={options}
        defaultValue={['a', 'b']}
        onValueChange={onValueChange}
      />,
    );

    const badgeRow = screen.getByText('Alpha').parentElement!;
    const removeIcon = badgeRow.querySelector('svg.lucide-circle-x');
    expect(removeIcon).toBeTruthy();

    fireEvent.click(removeIcon!);
    expect(onValueChange).toHaveBeenCalledWith(['b']);
  });

  it('clears disabled selected values via the clear icon', () => {
    const onValueChange = jest.fn();
    const { container } = render(
      <MultiSelect
        options={options}
        defaultValue={['a', 'b']}
        onValueChange={onValueChange}
      />,
    );

    fireEvent.click(container.querySelector('svg.lucide-x')!);
    expect(onValueChange).toHaveBeenCalledWith([]);
  });

  it('select all picks only enabled options', () => {
    const onValueChange = jest.fn();
    render(
      <MultiSelect
        options={options}
        defaultValue={[]}
        onValueChange={onValueChange}
      />,
    );

    fireEvent.click(screen.getByRole('button'));
    // i18next is not initialized in jsdom, so the select-all label renders
    // empty; it is always the first command item.
    fireEvent.click(document.querySelector('[cmdk-item]')!);
    expect(onValueChange).toHaveBeenCalledWith(['b']);
  });
});
