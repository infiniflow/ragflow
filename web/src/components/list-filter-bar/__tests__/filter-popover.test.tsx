import { fireEvent, render, screen, waitFor } from '@testing-library/react';
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

// A group whose options live in `filterGroup` (e.g. the dataset "system
// attributes" group) and a metadata group whose options are nested one level
// deeper (`metadata -> field -> values`).
const buildGroupedFilters = (): FilterCollection[] => [
  {
    field: 'type',
    label: 'Type',
    list: [
      { id: 'pdf', label: 'PDF', count: 1 },
      { id: 'docx', label: 'DOCX', count: 1 },
    ],
  },
  {
    field: 'metadata',
    label: 'Metadata',
    canSearch: true,
    list: [
      {
        id: 'file_name',
        field: 'file_name',
        label: 'file_name',
        list: [
          {
            id: 'report.pdf',
            field: 'report.pdf',
            label: 'report.pdf',
            count: 1,
          },
        ],
      },
    ],
  },
];

const groupedFilterGroup = { System: ['type'] };

// A status option and a metadata value share the id "3" — this happens with
// real data (e.g. run_status codes vs. numeric metadata values).
const buildCollidingFilters = (): FilterCollection[] => [
  {
    field: 'type',
    label: 'Type',
    list: [{ id: 'docx', label: 'DOCX', count: 1 }],
  },
  {
    field: 'run',
    label: 'Status',
    list: [
      { id: '3', label: '成功', count: 1 },
      { id: '4', label: '失败', count: 1 },
    ],
  },
  {
    field: 'metadata',
    label: 'Metadata',
    canSearch: true,
    list: [
      {
        id: 'q',
        field: 'q',
        label: 'q',
        list: [{ id: '3', field: '3', label: '3', count: 1 }],
      },
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

  it('expands a filter group that already has a selection', () => {
    render(
      <FilterPopover
        value={{ type: ['pdf'] }}
        filters={buildGroupedFilters()}
        filterGroup={groupedFilterGroup}
      >
        <button>open</button>
      </FilterPopover>,
    );
    fireEvent.click(screen.getByText('open'));

    // The selected option lives inside the collapsible "Type" group. Opening
    // the popover should reveal it instead of leaving the group collapsed.
    expect(screen.getByText('PDF')).toBeInTheDocument();
    expect(checkboxOf('PDF')).toBeChecked();
  });

  it('expands a nested metadata group that already has a selection', () => {
    render(
      <FilterPopover
        value={{ metadata: { file_name: ['report.pdf'] } }}
        filters={buildGroupedFilters()}
      >
        <button>open</button>
      </FilterPopover>,
    );
    fireEvent.click(screen.getByText('open'));

    expect(screen.getByText('report.pdf')).toBeInTheDocument();
    expect(checkboxOf('report.pdf')).toBeChecked();
  });

  it('does not duplicate a metadata value whose id matches a status option', async () => {
    const onChange = jest.fn();
    render(
      <FilterPopover
        value={{}}
        filters={buildCollidingFilters()}
        filterGroup={{ System: ['type', 'run'] }}
        onChange={onChange}
      >
        <button>open</button>
      </FilterPopover>,
    );
    fireEvent.click(screen.getByText('open'));

    // Expand the metadata field group "q" and select its only value "3".
    fireEvent.click(screen.getByText('q'));
    fireEvent.click(checkboxOf('3'));
    fireEvent.submit(document.querySelector('form')!);

    await waitFor(() => expect(onChange).toHaveBeenCalled());
    expect(onChange.mock.calls[0][0].metadata).toEqual({ q: ['3'] });
  });
});
