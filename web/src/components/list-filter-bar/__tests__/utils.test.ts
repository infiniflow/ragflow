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
