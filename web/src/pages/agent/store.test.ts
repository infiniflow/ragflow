import { Edge } from '@xyflow/react';
import { Operator } from './constant';
import useGraphStore from './store';

function baseNode(id: string, label: Operator) {
  return {
    id,
    type: 'ragNode',
    position: { x: 0, y: 0 },
    data: {
      label,
      name: id,
      form: {},
    },
  };
}

const createNode = (
  id: string,
  label: Operator,
  options: Partial<ReturnType<typeof baseNode>> = {},
) => ({
  ...baseNode(id, label),
  ...options,
});

const createEdge = (
  id: string,
  source: string,
  target: string,
  options: Partial<Edge> = {},
): Edge => ({
  id,
  source,
  target,
  ...options,
});

describe('useGraphStore.deleteIterationNodeById', () => {
  beforeEach(() => {
    useGraphStore.setState({
      nodes: [],
      edges: [],
      selectedNodeIds: [],
      selectedEdgeIds: [],
      clickedNodeId: '',
      clickedToolId: '',
    });
  });

  it('removes the iteration node, its descendants, and all incident edges', () => {
    const nodes = [
      createNode('begin', Operator.Begin),
      createNode('iteration:0', Operator.Iteration, { type: 'group' }),
      createNode('iterationStart:0', Operator.IterationStart, {
        parentId: 'iteration:0',
        type: 'iterationStartNode',
      }),
      createNode('message:0', Operator.Message, { parentId: 'iteration:0' }),
      createNode('message:1', Operator.Message, { parentId: 'message:0' }),
      createNode('generate:0', Operator.Generate),
    ];

    const edges = [
      createEdge('e1', 'begin', 'iteration:0'),
      createEdge('e2', 'iterationStart:0', 'message:0'),
      createEdge('e3', 'message:0', 'message:1'),
      createEdge('e4', 'message:0', 'generate:0'),
      createEdge('e5', 'generate:0', 'message:1'),
    ];

    useGraphStore.setState({
      nodes,
      edges,
      selectedNodeIds: ['iteration:0', 'message:0'],
      selectedEdgeIds: ['e2', 'e4'],
      clickedNodeId: 'message:0',
    });

    useGraphStore.getState().deleteIterationNodeById('iteration:0');

    const state = useGraphStore.getState();

    expect(state.nodes.map((node) => node.id)).toEqual(['begin', 'generate:0']);
    expect(state.edges.map((edge) => edge.id)).toEqual([]);
    expect(state.selectedNodeIds).toEqual([]);
    expect(state.selectedEdgeIds).toEqual([]);
    expect(state.clickedNodeId).toBe('');
  });

  it('preserves unrelated graph branches', () => {
    const nodes = [
      createNode('iteration:0', Operator.Iteration, { type: 'group' }),
      createNode('iterationStart:0', Operator.IterationStart, {
        parentId: 'iteration:0',
        type: 'iterationStartNode',
      }),
      createNode('message:0', Operator.Message, { parentId: 'iteration:0' }),
      createNode('begin', Operator.Begin),
      createNode('generate:0', Operator.Generate),
      createNode('message:2', Operator.Message),
    ];

    const edges = [
      createEdge('iteration-edge', 'iterationStart:0', 'message:0'),
      createEdge('branch-edge-a', 'begin', 'generate:0'),
      createEdge('branch-edge-b', 'generate:0', 'message:2'),
    ];

    useGraphStore.setState({ nodes, edges });

    useGraphStore.getState().deleteIterationNodeById('iteration:0');

    const state = useGraphStore.getState();

    expect(state.nodes.map((node) => node.id)).toEqual([
      'begin',
      'generate:0',
      'message:2',
    ]);
    expect(state.edges.map((edge) => edge.id)).toEqual([
      'branch-edge-a',
      'branch-edge-b',
    ]);
  });
});
