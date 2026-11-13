import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { renderHook } from '@testing-library/react';
import { Edge } from '@xyflow/react';
import { PropsWithChildren } from 'react';
import { Operator } from '../constant';
import { CanvasSnapshot, CanvasSnapshotContext } from '../context';
import useGraphStore from '../store';
import { useBuildUpstreamNodeOutputOptions } from './use-get-begin-query';
import { useSnapshotAwareGetNode } from './use-snapshot-graph';

// Regression: the version history preview reuses live canvas node components,
// whose variable resolution used to read the global canvas store (the current
// draft). Deleting a node from the draft then broke variable labels of every
// published version still referencing it. CanvasSnapshotContext redirects the
// resolution to the previewed version's own graph.

const AgentNodeId = 'Agent:HappyEyesJuggle';
const MessageNodeId = 'Message:CuriousPanda';

function makeNode(
  id: string,
  label: Operator,
  name: string,
  outputs: Record<string, any> = {},
): RAGFlowNodeType {
  return {
    id,
    position: { x: 0, y: 0 },
    data: { label, name, form: { outputs } },
  } as RAGFlowNodeType;
}

const AgentNode = makeNode(AgentNodeId, Operator.Agent, 'glm-4_jin', {
  text: { type: 'string' },
});
const MessageNode = makeNode(MessageNodeId, Operator.Message, 'reply');

const SnapshotEdge: Edge = {
  id: 'edge-agent-message',
  source: AgentNodeId,
  target: MessageNodeId,
};

// The version being previewed: Begin -> Agent -> Message.
const PreviewSnapshot: CanvasSnapshot = {
  nodes: [AgentNode, MessageNode],
  edges: [SnapshotEdge],
  globals: {},
  variables: {},
};

function renderWithSnapshot<T>(useHook: () => T) {
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <CanvasSnapshotContext.Provider value={PreviewSnapshot}>
        {children}
      </CanvasSnapshotContext.Provider>
    );
  }
  return renderHook(useHook, { wrapper: Wrapper });
}

beforeEach(() => {
  // The current draft: the Agent node was deleted, only Message remains.
  useGraphStore.setState({ nodes: [MessageNode], edges: [] });
});

describe('useSnapshotAwareGetNode', () => {
  it('resolves nodes from the live canvas store without a snapshot', () => {
    const { result } = renderHook(() => useSnapshotAwareGetNode());

    expect(result.current(MessageNodeId)?.id).toBe(MessageNodeId);
    expect(result.current(AgentNodeId)).toBeUndefined();
  });

  it('resolves nodes from the snapshot when one is provided', () => {
    const { result } = renderWithSnapshot(() => useSnapshotAwareGetNode());

    // The draft has no Agent node; the snapshot does.
    expect(result.current(AgentNodeId)?.data.name).toBe('glm-4_jin');
  });
});

describe('useBuildUpstreamNodeOutputOptions', () => {
  it('finds no upstream outputs in the current draft', () => {
    const { result } = renderHook(() =>
      useBuildUpstreamNodeOutputOptions(MessageNodeId),
    );

    expect(result.current).toEqual([]);
  });

  it('resolves upstream outputs from the snapshot graph', () => {
    const { result } = renderWithSnapshot(() =>
      useBuildUpstreamNodeOutputOptions(MessageNodeId),
    );

    const values = result.current.flatMap((group) =>
      group.options.map((option) => option.value),
    );
    expect(values).toContain(`${AgentNodeId}@text`);
  });
});
