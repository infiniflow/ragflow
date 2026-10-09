import {
  shouldShowBuiltin,
  filterBuiltinByKeyword,
  toBuiltinListItem,
} from './builtin-pipeline-list';
import { AgentCategory } from '@/constants/agent';
import { AgentListItemType, IBuiltinPipeline } from '@/interfaces/database/agent';

const sample: IBuiltinPipeline[] = [
  {
    id: 'general',
    title: 'General',
    description: 'Default parsing method',
    filename: 'ingestion_pipeline_general.json',
  },
  {
    id: 'book',
    title: 'Book',
    description: 'Long document parsing',
    filename: 'ingestion_pipeline_book.json',
  },
  {
    id: 'naive',
    title: 'Naive',
    description: undefined,
    filename: 'ingestion_pipeline_naive.json',
  },
];

describe('shouldShowBuiltin', () => {
  it('shows builtin in the "All" view (no category filter)', () => {
    expect(shouldShowBuiltin(undefined)).toBe(true);
    expect(shouldShowBuiltin([])).toBe(true);
  });

  it('shows builtin in the Pipeline (dataflow) view', () => {
    expect(shouldShowBuiltin([AgentCategory.DataflowCanvas])).toBe(true);
  });

  it('shows builtin when dataflow is among several selected categories', () => {
    expect(
      shouldShowBuiltin([
        AgentCategory.AgentCanvas,
        AgentCategory.DataflowCanvas,
      ]),
    ).toBe(true);
  });

  it('hides builtin in the pure Agent view', () => {
    expect(shouldShowBuiltin([AgentCategory.AgentCanvas])).toBe(false);
  });

  it('hides builtin in the compilation template group view', () => {
    expect(shouldShowBuiltin(['compilation_template_group'])).toBe(false);
  });

  it('hides builtin when only non-dataflow categories are selected', () => {
    expect(
      shouldShowBuiltin([
        AgentCategory.AgentCanvas,
        'compilation_template_group',
      ]),
    ).toBe(false);
  });
});

describe('filterBuiltinByKeyword', () => {
  it('returns all items when the keyword is empty/undefined', () => {
    expect(filterBuiltinByKeyword(sample, undefined)).toHaveLength(3);
    expect(filterBuiltinByKeyword(sample, '')).toHaveLength(3);
    expect(filterBuiltinByKeyword(sample, '   ')).toHaveLength(3);
  });

  it('matches case-insensitively against the title', () => {
    const result = filterBuiltinByKeyword(sample, 'BOOK');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('book');
  });

  it('matches against the description', () => {
    const result = filterBuiltinByKeyword(sample, 'long document');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('book');
  });

  it('matches a partial substring', () => {
    const result = filterBuiltinByKeyword(sample, 'parsing');
    // "General" -> "Default parsing method", "Book" -> "Long document parsing"
    expect(result.map((r) => r.id).sort()).toEqual(['book', 'general']);
  });

  it('returns an empty list when nothing matches', () => {
    expect(filterBuiltinByKeyword(sample, 'zzz-no-match')).toEqual([]);
  });

  it('does not throw on items with undefined description', () => {
    expect(() => filterBuiltinByKeyword(sample, 'naive')).not.toThrow();
    const result = filterBuiltinByKeyword(sample, 'naive');
    expect(result[0].id).toBe('naive');
  });
});

describe('toBuiltinListItem', () => {
  it('tags the item as a read-only builtin dataflow canvas', () => {
    const item = toBuiltinListItem(sample[0]);
    expect(item.type).toBe(AgentListItemType.BuiltinPipeline);
    expect(item.builtin).toBe(true);
    expect(item.canvas_category).toBe(AgentCategory.DataflowCanvas);
  });

  it('preserves the source fields', () => {
    const item = toBuiltinListItem(sample[0]);
    expect(item.id).toBe('general');
    expect(item.title).toBe('General');
    expect(item.description).toBe('Default parsing method');
    expect(item.filename).toBe('ingestion_pipeline_general.json');
  });

  it('keeps an undefined description as undefined', () => {
    const item = toBuiltinListItem(sample[2]);
    expect(item.description).toBeUndefined();
  });
});
