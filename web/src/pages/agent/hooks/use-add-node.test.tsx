import { act, renderHook } from '@testing-library/react';
import { Position } from '@xyflow/react';
import { NodeHandleId, Operator } from '../constant';
import useGraphStore from '../store';
import { useAddNode } from './use-add-node';

// This file uses jest.mock, so it is transformed by esbuild-jest's babel
// pass: imported bindings must not be used in type positions (type-only
// imports break the pass), but annotations over global types are fine.

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchDefaultModelDictionary: () => ({ llm_id: '' }),
}));

// The global human-id mock returns the same id for every call, which would
// mask duplicate-id regressions in the group start nodes; make ids unique.
jest.mock('human-id', () => {
  let counter = 0;
  return () => `human-id-${++counter}`;
});

const createSwitchNode = () => ({
  id: 'switch:0',
  type: 'switchNode',
  position: { x: 0, y: 0 },
  data: { label: Operator.Switch, name: 'switch_0', form: {} },
});

describe('useAddNode', () => {
  beforeEach(() => {
    useGraphStore.setState({ nodes: [], edges: [] });
  });

  it('connects a new iteration node to the switch branch handle that opened the dropdown', () => {
    useGraphStore.setState({ nodes: [createSwitchNode()], edges: [] });

    const { result } = renderHook(() => useAddNode());

    let newNodeId: string | undefined;
    act(() => {
      newNodeId = result.current.addCanvasNode(Operator.Iteration, {
        nodeId: 'switch:0',
        id: 'Case 1',
        position: Position.Right,
        isFromConnectionDrag: false,
      })();
    });

    const { nodes, edges } = useGraphStore.getState();

    expect(nodes.some((x) => x.id === newNodeId)).toBe(true);
    // The iteration start child is created inside the group
    expect(
      nodes.some(
        (x) =>
          x.data.label === Operator.IterationStart && x.parentId === newNodeId,
      ),
    ).toBe(true);

    expect(edges).toHaveLength(1);
    expect(edges[0]).toMatchObject({
      source: 'switch:0',
      target: newNodeId,
      sourceHandle: 'Case 1',
      targetHandle: NodeHandleId.End,
    });

    // The group node must not overlap its parent: with nodeOrigin [0.5, 0]
    // position.x is the node's center, so its left edge is x - width / 2.
    // The switch node at x = 0 with the default 200 width has its right
    // edge at 100, plus the 100 gap the left edge must be at 200.
    const iterationNode = nodes.find((x) => x.id === newNodeId);
    expect(iterationNode!.position.x - (iterationNode!.width ?? 0) / 2).toBe(
      200,
    );
  });

  it('stacks the next sibling below a group child without overlap', () => {
    useGraphStore.setState({ nodes: [createSwitchNode()], edges: [] });

    const { result } = renderHook(() => useAddNode());

    let iterationId: string | undefined;
    act(() => {
      iterationId = result.current.addCanvasNode(Operator.Iteration, {
        nodeId: 'switch:0',
        id: 'Case 1',
        position: Position.Right,
        isFromConnectionDrag: false,
      })();
    });

    let messageId: string | undefined;
    act(() => {
      messageId = result.current.addCanvasNode(Operator.Message, {
        nodeId: 'switch:0',
        id: 'Case 1',
        position: Position.Right,
        isFromConnectionDrag: false,
      })();
    });

    const { nodes } = useGraphStore.getState();
    const iterationNode = nodes.find((x) => x.id === iterationId);
    const messageNode = nodes.find((x) => x.id === messageId);
    // A group child is 250 tall; the +150 stacking step of plain nodes
    // would land inside it.
    expect(messageNode!.position.y).toBeGreaterThanOrEqual(
      iterationNode!.position.y + (iterationNode!.height ?? 0),
    );
  });

  it('falls back to the start handle when no specific handle was picked', () => {
    useGraphStore.setState({ nodes: [createSwitchNode()], edges: [] });

    const { result } = renderHook(() => useAddNode());

    let newNodeId: string | undefined;
    act(() => {
      newNodeId = result.current.addCanvasNode(Operator.Loop, {
        nodeId: 'switch:0',
        position: Position.Right,
        isFromConnectionDrag: false,
      })();
    });

    const { edges } = useGraphStore.getState();

    expect(edges).toHaveLength(1);
    expect(edges[0]).toMatchObject({
      source: 'switch:0',
      target: newNodeId,
      sourceHandle: NodeHandleId.Start,
      targetHandle: NodeHandleId.End,
    });
  });

  it('adds a tool node only once when the same stale callback fires twice', () => {
    // Mirrors the React StrictMode double-invoked mount effect in
    // ToolPopover: both invocations share one render's closure, so the
    // second call must consult fresh store state to see the edge the
    // first call just committed. Otherwise two tool nodes stack at the
    // same position below the agent.
    const agentNode = {
      id: 'Agent:0',
      type: 'agentNode',
      position: { x: 100, y: 100 },
      data: {
        label: Operator.Agent,
        name: 'agent_0',
        form: { tools: [{ component_name: Operator.Retrieval }] },
      },
    };
    useGraphStore.setState({ nodes: [agentNode], edges: [] });

    const { result } = renderHook(() => useAddNode());

    const addTool = result.current.addCanvasNode(Operator.Tool, {
      nodeId: 'Agent:0',
      position: Position.Bottom,
    });

    act(() => {
      addTool();
      addTool();
    });

    const { nodes, edges } = useGraphStore.getState();
    expect(nodes.filter((x) => x.data.label === Operator.Tool)).toHaveLength(1);
    expect(
      edges.filter((x) => x.sourceHandle === NodeHandleId.Tool),
    ).toHaveLength(1);
  });

  it('adds an iteration node again after the previous one was deleted', () => {
    useGraphStore.setState({ nodes: [createSwitchNode()], edges: [] });

    const { result } = renderHook(() => useAddNode());

    const addIteration = () => {
      let newNodeId: string | undefined;
      act(() => {
        newNodeId = result.current.addCanvasNode(Operator.Iteration, {
          nodeId: 'switch:0',
          id: 'end_cpn_ids',
          position: Position.Right,
          isFromConnectionDrag: false,
        })();
      });
      return newNodeId;
    };

    const firstId = addIteration();
    act(() => {
      useGraphStore.getState().deleteIterationNodeById(firstId!);
    });

    expect(useGraphStore.getState().nodes).toHaveLength(1);
    expect(useGraphStore.getState().edges).toHaveLength(0);

    const secondId = addIteration();
    expect(secondId).not.toBe(firstId);

    const { nodes, edges } = useGraphStore.getState();
    expect(nodes.some((x) => x.id === secondId)).toBe(true);
    // The new group gets its own start child bound to it — not a reused
    // (frozen) object still pointing at the deleted iteration node
    const startChildren = nodes.filter(
      (x) => x.data.label === Operator.IterationStart,
    );
    expect(startChildren).toHaveLength(1);
    expect(startChildren[0].parentId).toBe(secondId);
    expect(edges).toHaveLength(1);
    expect(edges[0]).toMatchObject({
      source: 'switch:0',
      target: secondId,
      sourceHandle: 'end_cpn_ids',
      targetHandle: NodeHandleId.End,
    });
  });
});
