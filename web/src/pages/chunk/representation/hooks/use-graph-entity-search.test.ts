import { useFetchDocumentStructureGraph } from '@/hooks/use-document-request';
import { act, renderHook } from '@testing-library/react';
import { useGraphEntitySearch } from './use-graph-entity-search';

jest.mock('@/hooks/use-document-request', () => ({
  useFetchDocumentStructureGraph: jest.fn(),
}));

const mockFetchDocumentStructureGraph = jest.mocked(
  useFetchDocumentStructureGraph,
);

function setupHook(entities: Array<Record<string, unknown>>) {
  mockFetchDocumentStructureGraph.mockReturnValue({
    data: {
      templates: [
        {
          template_id: 't1',
          template_name: 'Graph',
          kind: 'knowledge_graph',
          entities,
          relations: [],
        },
      ],
    },
    loading: false,
  } as never);

  return renderHook(() => useGraphEntitySearch());
}

describe('useGraphEntitySearch handleNoMatchEnter', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('treats Enter on an existing entity name like a dropdown selection: highlight + keyword refetch', () => {
    const { result } = setupHook([
      { id: 'e1', name: 'swallow' },
      { id: 'e2', name: 'owl tree' },
    ]);

    act(() => {
      result.current.handleNoMatchEnter('swallow');
    });

    expect(result.current.highlightNodeId).toBe('swallow');
    expect(result.current.graphSelectValue).toBe('swallow');
    // The keyword subgraph refetch is armed with the entity name, so the
    // graph and the entity count refresh from the server.
    expect(mockFetchDocumentStructureGraph).toHaveBeenLastCalledWith('swallow');
  });

  it('matches entity names case-insensitively and returns the canonical name', () => {
    const { result } = setupHook([
      { id: 'e1', name: 'Swan', aliases: ['swan lake'] },
    ]);

    act(() => {
      result.current.handleNoMatchEnter('  SWAN ');
    });

    expect(result.current.highlightNodeId).toBe('Swan');
  });

  it('matches an alias and highlights the owning entity', () => {
    const { result } = setupHook([
      { id: 'e1', name: 'Swan', aliases: ['swan lake'] },
    ]);

    act(() => {
      result.current.handleNoMatchEnter('swan lake');
    });

    expect(result.current.highlightNodeId).toBe('Swan');
  });

  it('falls back to the server-side keyword search for text that names no entity', () => {
    const { result } = setupHook([{ id: 'e1', name: 'swallow' }]);

    act(() => {
      result.current.handleNoMatchEnter('something else');
    });

    expect(result.current.highlightNodeId).toBeNull();
    expect(result.current.graphSelectValue).toBe('something else');
    expect(mockFetchDocumentStructureGraph).toHaveBeenLastCalledWith(
      'something else',
    );
  });
});

describe('useGraphEntitySearch handleSelectEntity', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('refetches the keyword subgraph for the picked entity and keeps it highlighted', () => {
    const { result } = setupHook([
      { id: 'e1', name: 'swallow' },
      { id: 'e2', name: 'owl tree' },
    ]);

    act(() => {
      result.current.handleSelectEntity('owl tree');
    });

    expect(result.current.highlightNodeId).toBe('owl tree');
    expect(result.current.graphSelectValue).toBe('owl tree');
    expect(mockFetchDocumentStructureGraph).toHaveBeenLastCalledWith(
      'owl tree',
    );
  });

  it('clearing the selection refetches the full graph', () => {
    const { result } = setupHook([{ id: 'e1', name: 'swallow' }]);

    act(() => {
      result.current.handleSelectEntity('swallow');
    });
    act(() => {
      result.current.handleSelectEntity('');
    });

    expect(result.current.highlightNodeId).toBeNull();
    expect(result.current.graphSelectValue).toBe('');
    expect(mockFetchDocumentStructureGraph).toHaveBeenLastCalledWith('');
  });
});

describe('useGraphEntitySearch keyword channel sync', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  // Repro: Enter a keyword with no match (graphKeywords='sun'), then type an
  // existing entity into the expandable search input that takes over when the
  // result kind is not a graph. The select must show the keyword that actually
  // drives the current subgraph, not the stale failed one.
  it('shows the expandable-input keyword that drives the fetch, not the stale graph keyword', () => {
    const { result } = setupHook([{ id: 'e1', name: 'swallow' }]);

    act(() => {
      result.current.handleNoMatchEnter('sun');
    });
    expect(result.current.graphSelectValue).toBe('sun');

    // The debounce timer is scheduled by an effect after the state-change
    // render, so the keystroke and the timer advance need separate acts.
    act(() => {
      result.current.handleSearchKeywordChange('乾坤');
    });
    act(() => {
      jest.advanceTimersByTime(500);
    });

    expect(mockFetchDocumentStructureGraph).toHaveBeenLastCalledWith('乾坤');
    expect(result.current.graphSelectValue).toBe('乾坤');
  });

  // The two keyword sources must stay mutually exclusive: after an
  // expandable-input search, picking an entity has to re-arm the fetch with
  // the entity name instead of staying shadowed by the old searchKeyword.
  it('picking an entity re-arms the fetch with the entity name after an expandable-input search', () => {
    const { result } = setupHook([
      { id: 'e1', name: 'swallow' },
      { id: 'e2', name: '乾坤' },
    ]);

    act(() => {
      result.current.handleSearchKeywordChange('乾坤');
    });
    act(() => {
      jest.advanceTimersByTime(500);
    });
    expect(mockFetchDocumentStructureGraph).toHaveBeenLastCalledWith('乾坤');

    act(() => {
      result.current.handleSelectEntity('swallow');
    });
    // The cleared searchKeyword flushes through its debounce before the
    // graph keyword drives the fetch again.
    act(() => {
      jest.advanceTimersByTime(500);
    });

    expect(mockFetchDocumentStructureGraph).toHaveBeenLastCalledWith('swallow');
    expect(result.current.graphSelectValue).toBe('swallow');
  });
});
