import { renderHook } from '@testing-library/react';
import useGraphStore from '../agent/store';
import { useFetchBuiltinPipelineDataOnMount } from './use-fetch-builtin-pipeline-data';

// Referenced from jest.mock factories, so the name must start with "mock".
const mockBuiltinDsl = {
  graph: {
    nodes: [
      {
        id: 'File',
        type: 'fileNode',
        position: { x: 0, y: 0 },
        data: { label: 'File', name: 'File', form: {} },
      },
      {
        id: 'Parser:1',
        type: 'parserNode',
        position: { x: 100, y: 0 },
        data: { label: 'Parser', name: 'Parser', form: {} },
      },
    ],
    edges: [
      {
        id: 'edge-1',
        source: 'File',
        target: 'Parser:1',
      },
    ],
  },
};

jest.mock('react-router', () => ({
  useParams: () => ({ id: 'general' }),
}));

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchPipelineDslByPipelineId: (_id: string, isBuiltin: boolean) => ({
    dsl: isBuiltin ? mockBuiltinDsl : {},
    loading: false,
  }),
}));

describe('useFetchBuiltinPipelineDataOnMount', () => {
  beforeEach(() => {
    useGraphStore.setState({
      nodes: [],
      edges: [],
      clickedNodeId: '',
    });
  });

  it('loads the builtin DSL graph into the shared store', () => {
    renderHook(() => useFetchBuiltinPipelineDataOnMount());

    const { nodes, edges } = useGraphStore.getState();
    expect(nodes.map((x) => x.id)).toEqual(['File', 'Parser:1']);
    expect(edges.map((x) => x.id)).toEqual(['edge-1']);
  });

  it('empties the store and the clicked node on unmount', () => {
    const { unmount } = renderHook(() => useFetchBuiltinPipelineDataOnMount());

    expect(useGraphStore.getState().nodes).toHaveLength(2);
    unmount();

    const { nodes, edges, clickedNodeId } = useGraphStore.getState();
    expect(nodes).toEqual([]);
    expect(edges).toEqual([]);
    expect(clickedNodeId).toBe('');
  });
});
