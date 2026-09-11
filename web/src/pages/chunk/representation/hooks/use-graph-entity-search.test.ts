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

  it('treats Enter on an existing entity name like a dropdown selection: highlight + dim, no keyword refetch', () => {
    const { result } = setupHook([
      { id: 'e1', name: 'swallow' },
      { id: 'e2', name: 'owl tree' },
    ]);

    act(() => {
      result.current.handleNoMatchEnter('swallow');
    });

    expect(result.current.highlightNodeId).toBe('swallow');
    expect(result.current.graphSelectValue).toBe('swallow');
    // The keyword subgraph refetch must NOT be armed for a name hit.
    expect(mockFetchDocumentStructureGraph).toHaveBeenLastCalledWith('');
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
