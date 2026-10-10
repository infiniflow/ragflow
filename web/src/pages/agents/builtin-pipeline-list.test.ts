import {
  shouldShowBuiltinForRaw,
  filterBuiltinByKeyword,
  toBuiltinListItem,
  resolveAgentsEmptyState,
} from './builtin-pipeline-list';
import { AgentCategory } from '@/constants/agent';
import { AgentListItemType } from '@/interfaces/database/agent';

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchBuiltinPipelines: jest.fn(),
}));

const sample = [
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

describe('shouldShowBuiltinForRaw', () => {
  it('shows builtin in the "All" view (no category filter)', () => {
    expect(shouldShowBuiltinForRaw(undefined)).toBe(true);
  });

  it('shows builtin in the Pipeline (dataflow) view', () => {
    expect(shouldShowBuiltinForRaw(AgentCategory.DataflowCanvas)).toBe(true);
    expect(shouldShowBuiltinForRaw([AgentCategory.DataflowCanvas])).toBe(true);
  });

  it('shows builtin when dataflow is among several selected categories', () => {
    expect(
      shouldShowBuiltinForRaw([
        AgentCategory.AgentCanvas,
        AgentCategory.DataflowCanvas,
      ]),
    ).toBe(true);
  });

  it('hides builtin in the pure Agent view', () => {
    expect(shouldShowBuiltinForRaw(AgentCategory.AgentCanvas)).toBe(false);
    expect(shouldShowBuiltinForRaw([AgentCategory.AgentCanvas])).toBe(false);
  });

  it('hides builtin in the compilation template group view', () => {
    expect(shouldShowBuiltinForRaw('compilation_template_group')).toBe(false);
    expect(shouldShowBuiltinForRaw(['compilation_template_group'])).toBe(false);
  });

  it('hides builtin when only non-dataflow categories are selected', () => {
    expect(
      shouldShowBuiltinForRaw([
        AgentCategory.AgentCanvas,
        'compilation_template_group',
      ]),
    ).toBe(false);
  });

  it('hides builtin for an array carrying no plain ids', () => {
    // An array of structured filters has no plain category ids, so the
    // "All"/"Pipeline" intent cannot be assumed and built-ins stay hidden.
    expect(
      shouldShowBuiltinForRaw([
        { operator: 'or', values: [] } as unknown as string,
      ]),
    ).toBe(false);
  });

  it('hides builtin for a structured (non-string) category filter', () => {
    // A Record-typed filter carries no plain ids; the "All"/"Pipeline" intent
    // cannot be assumed, so built-ins stay hidden rather than guessing.
    expect(
      shouldShowBuiltinForRaw({
        operator: 'or',
        values: [AgentCategory.DataflowCanvas],
      } as any),
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

describe('resolveAgentsEmptyState', () => {
  const base = {
    dataLength: 0,
    builtinItemsLength: 0,
    searchString: '',
    listLoading: false,
    builtinVisible: true,
    builtinLoading: false,
  };

  it('shows the empty card (no search) when there is nothing at all', () => {
    expect(resolveAgentsEmptyState(base)).toBe('empty');
  });

  it('shows the search empty card when a search yields no user or builtin hits', () => {
    expect(
      resolveAgentsEmptyState({ ...base, searchString: 'zzz-no-match' }),
    ).toBe('search-empty');
  });

  it('renders content when user items exist even with no search', () => {
    expect(resolveAgentsEmptyState({ ...base, dataLength: 3 })).toBe('content');
  });

  it('renders content when filtered builtin items exist (the zero-match-search regression)', () => {
    // Category-eligible view (builtinVisible true) + search that did not match
    // anything would previously leave a blank CardContainer; the matched
    // builtin count now drives the decision.
    expect(
      resolveAgentsEmptyState({
        ...base,
        searchString: 'zzz-no-match',
        builtinItemsLength: 2,
      }),
    ).toBe('content');
  });

  it('keeps the container mounted while the eligible builtin catalog loads', () => {
    expect(resolveAgentsEmptyState({ ...base, builtinLoading: true })).toBe(
      'content',
    );
  });

  it('renders nothing while the user list is loading', () => {
    expect(resolveAgentsEmptyState({ ...base, listLoading: true })).toBe(
      'loading',
    );
  });

  it('treats a non-eligible (Agent) view as if builtins were absent', () => {
    // builtinVisible false => the builtin probe is not mounted, so the reported
    // builtin count stays 0; with no user items and a search, it is a
    // search-empty, not content.
    expect(
      resolveAgentsEmptyState({
        ...base,
        builtinVisible: false,
        searchString: 'zzz-no-match',
        builtinItemsLength: 0,
      }),
    ).toBe('search-empty');
  });
});
