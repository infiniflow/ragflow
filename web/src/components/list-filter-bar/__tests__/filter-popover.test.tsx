import { fireEvent, render, screen } from '@testing-library/react';
import { FilterPopover } from '../filter-popover';
import { FilterCollection } from '../interface';

const buildFilters = (pdfCount: number): FilterCollection[] => [
  {
    field: 'type',
    label: 'Type',
    list: [
      { id: 'pdf', label: 'PDF', count: pdfCount },
      { id: 'docx', label: 'DOCX', count: 1 },
    ],
  },
];

// jsdom does not provide ResizeObserver, which the Radix popover relies on.
beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

// The checkbox has no accessible name; it sits next to its label text.
const checkboxOf = (label: string) =>
  screen.getByText(label).parentElement!.querySelector('[role="checkbox"]')!;

describe('FilterPopover', () => {
  it('keeps unsubmitted selections when the filter options refresh', () => {
    const value = { type: [] };
    const { rerender } = render(
      <FilterPopover value={value} filters={buildFilters(3)}>
        <button>open</button>
      </FilterPopover>,
    );
    fireEvent.click(screen.getByText('open'));

    const pdf = checkboxOf('PDF');
    fireEvent.click(pdf);
    expect(pdf).toBeChecked();

    // The document list poll refetches the options with new counts.
    rerender(
      <FilterPopover value={value} filters={buildFilters(4)}>
        <button>open</button>
      </FilterPopover>,
    );

    expect(checkboxOf('PDF')).toBeChecked();
  });

  it('takes a newly applied value', () => {
    const { rerender } = render(
      <FilterPopover value={{ type: [] }} filters={buildFilters(3)}>
        <button>open</button>
      </FilterPopover>,
    );
    fireEvent.click(screen.getByText('open'));
    expect(checkboxOf('DOCX')).not.toBeChecked();

    rerender(
      <FilterPopover value={{ type: ['docx'] }} filters={buildFilters(3)}>
        <button>open</button>
      </FilterPopover>,
    );

    expect(checkboxOf('DOCX')).toBeChecked();
  });
});
