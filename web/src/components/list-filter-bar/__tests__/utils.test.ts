import { FilterCollection, FilterValue } from '../interface';
import { pruneFilterValue } from '../utils';

const Filters: FilterCollection[] = [
  { field: 'type', label: 'Type', list: [{ id: 'pdf', label: 'PDF' }] },
  {
    field: 'metadata',
    label: 'Metadata',
    list: [
      { id: 'author', label: 'Author', list: [{ id: 'Ada', label: 'Ada' }] },
    ],
  },
];

it('prunes missing simple and nested selections', () => {
  expect(
    pruneFilterValue(
      {
        type: ['pdf', 'docx'],
        metadata: { author: ['Ada', 'Grace'] },
      },
      Filters,
    ),
  ).toEqual({ type: ['pdf'], metadata: { author: ['Ada'] } });
});

it('keeps the value reference when counts change without removing selections', () => {
  const value: FilterValue = { type: ['pdf'], metadata: { author: ['Ada'] } };
  expect(pruneFilterValue(value, Filters)).toBe(value);
});

it('keeps selections before filter options have loaded', () => {
  const value = { type: ['pdf'] };
  expect(pruneFilterValue(value, [])).toBe(value);
  expect(
    pruneFilterValue(value, [
      { field: 'type', label: 'Type', list: undefined! },
    ]),
  ).toBe(value);
});

it('does not keep a removed type because its ID exists in metadata', () => {
  expect(
    pruneFilterValue({ type: ['pdf'] }, [
      { field: 'type', label: 'Type', list: [] },
      {
        field: 'metadata',
        label: 'Metadata',
        list: [
          {
            id: 'format',
            label: 'Format',
            list: [{ id: 'pdf', label: 'PDF' }],
          },
        ],
      },
    ]),
  ).toEqual({ type: [] });
});

it('checks each metadata key against its own values', () => {
  expect(
    pruneFilterValue({ metadata: { author: ['Ada'], reviewer: ['Ada'] } }, [
      {
        field: 'metadata',
        label: 'Metadata',
        list: [
          {
            id: 'author',
            label: 'Author',
            list: [{ id: 'Grace', label: 'Grace' }],
          },
          {
            id: 'reviewer',
            label: 'Reviewer',
            list: [{ id: 'Ada', label: 'Ada' }],
          },
        ],
      },
    ]),
  ).toEqual({ metadata: { author: [], reviewer: ['Ada'] } });
});

it('clears active values after an empty catalog has loaded', () => {
  expect(
    pruneFilterValue(
      { type: ['pdf'], run: ['3'], metadata: { author: ['Ada'] } },
      [],
      true,
    ),
  ).toEqual({});
});

it('keeps an already empty value reference after an empty catalog has loaded', () => {
  const value = { type: [], metadata: { author: [] } };
  expect(pruneFilterValue(value, [], true)).toBe(value);
});
