import { Edge } from '@xyflow/react';
import { NodeHandleId, Operator } from './constant';
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

  it('removes agent tool chains nested inside an iteration subtree', () => {
    const nodes = [
      createNode('iteration:0', Operator.Iteration, { type: 'group' }),
      createNode('iterationStart:0', Operator.IterationStart, {
        parentId: 'iteration:0',
        type: 'iterationStartNode',
      }),
      createNode('agent:0', Operator.Agent, { parentId: 'iteration:0' }),
      createNode('tool:0', Operator.Tool),
      createNode('message:0', Operator.Message),
      createNode('begin', Operator.Begin),
      createNode('generate:0', Operator.Generate),
    ];

    const edges = [
      createEdge('iteration-edge', 'iterationStart:0', 'agent:0'),
      createEdge('tool-edge', 'agent:0', 'tool:0', {
        sourceHandle: NodeHandleId.AgentBottom,
      }),
      createEdge('tool-output-edge', 'tool:0', 'message:0', {
        sourceHandle: NodeHandleId.Tool,
      }),
      createEdge('branch-edge', 'begin', 'generate:0'),
    ];

    useGraphStore.setState({ nodes, edges });

    useGraphStore.getState().deleteIterationNodeById('iteration:0');

    const state = useGraphStore.getState();

    expect(state.nodes.map((node) => node.id)).toEqual(['begin', 'generate:0']);
    expect(state.edges.map((edge) => edge.id)).toEqual(['branch-edge']);
  });
});

describe('useGraphStore.toggleBottomCollapse', () => {
  const buildSubgraph = () => {
    const nodes = [
      createNode('agent:0', Operator.Agent),
      createNode('agent:1', Operator.Agent),
      createNode('tool:0', Operator.Tool),
      createNode('tool:1', Operator.Tool),
      createNode('message:0', Operator.Message),
    ];

    const edges = [
      createEdge('e-sub', 'agent:0', 'agent:1', {
        sourceHandle: NodeHandleId.AgentBottom,
        targetHandle: NodeHandleId.AgentTop,
      }),
      createEdge('e-tool-0', 'agent:0', 'tool:0', {
        sourceHandle: NodeHandleId.Tool,
      }),
      createEdge('e-tool-1', 'agent:1', 'tool:1', {
        sourceHandle: NodeHandleId.Tool,
      }),
      createEdge('e-goto', 'agent:1', 'message:0', {
        sourceHandle: NodeHandleId.AgentException,
      }),
      createEdge('e-main', 'agent:0', 'message:0', {
        sourceHandle: NodeHandleId.Start,
      }),
    ];

    return { nodes, edges };
  };

  const hiddenIds = (items: { id: string; hidden?: boolean }[]) =>
    items
      .filter((x) => x.hidden)
      .map((x) => x.id)
      .sort();

  beforeEach(() => {
    useGraphStore.setState({
      nodes: [],
      edges: [],
      selectedNodeIds: [],
      selectedEdgeIds: [],
      clickedNodeId: '',
      clickedToolId: '',
      collapsedBottomHandles: {},
    });
  });

  it('hides the handle subtree and restores it on a second toggle', () => {
    const { nodes, edges } = buildSubgraph();

    useGraphStore.setState({
      nodes,
      edges,
      selectedNodeIds: ['agent:1'],
      selectedEdgeIds: ['e-tool-1'],
    });

    useGraphStore
      .getState()
      .toggleBottomCollapse('agent:0', NodeHandleId.AgentBottom);

    let state = useGraphStore.getState();
    expect(state.collapsedBottomHandles).toEqual({
      'agent:0': [NodeHandleId.AgentBottom],
    });
    // The sub-agent's whole subtree is hidden, including its tool node
    expect(hiddenIds(state.nodes)).toEqual(['agent:1', 'tool:1']);
    // The goto edge from a hidden node is hidden too,
    // while the right-side main-flow edge stays visible
    expect(hiddenIds(state.edges)).toEqual(['e-goto', 'e-sub', 'e-tool-1']);
    expect(state.selectedNodeIds).toEqual([]);
    expect(state.selectedEdgeIds).toEqual([]);

    useGraphStore
      .getState()
      .toggleBottomCollapse('agent:0', NodeHandleId.AgentBottom);

    state = useGraphStore.getState();
    expect(state.collapsedBottomHandles).toEqual({});
    expect(state.nodes.every((x) => !x.hidden)).toBe(true);
    expect(state.edges.every((x) => !x.hidden)).toBe(true);
  });

  it('collapses the two bottom handles independently', () => {
    const { nodes, edges } = buildSubgraph();

    useGraphStore.setState({ nodes, edges });

    useGraphStore
      .getState()
      .toggleBottomCollapse('agent:0', NodeHandleId.Tool);

    let state = useGraphStore.getState();
    // Only the tool node is hidden; the sub-agent subtree stays visible
    expect(hiddenIds(state.nodes)).toEqual(['tool:0']);
    expect(hiddenIds(state.edges)).toEqual(['e-tool-0']);

    useGraphStore
      .getState()
      .toggleBottomCollapse('agent:0', NodeHandleId.AgentBottom);

    state = useGraphStore.getState();
    expect(hiddenIds(state.nodes)).toEqual(['agent:1', 'tool:0', 'tool:1']);
    expect(hiddenIds(state.edges)).toEqual([
      'e-goto',
      'e-sub',
      'e-tool-0',
      'e-tool-1',
    ]);

    // Expanding one handle keeps the other handle's subtree hidden
    useGraphStore
      .getState()
      .toggleBottomCollapse('agent:0', NodeHandleId.Tool);

    state = useGraphStore.getState();
    expect(hiddenIds(state.nodes)).toEqual(['agent:1', 'tool:1']);
    expect(hiddenIds(state.edges)).toEqual(['e-goto', 'e-sub', 'e-tool-1']);
  });

  it('does nothing when there is no bottom subtree', () => {
    useGraphStore.setState({
      nodes: [createNode('agent:0', Operator.Agent)],
      edges: [],
    });

    useGraphStore
      .getState()
      .toggleBottomCollapse('agent:0', NodeHandleId.AgentBottom);

    expect(useGraphStore.getState().collapsedBottomHandles).toEqual({});
  });
});
