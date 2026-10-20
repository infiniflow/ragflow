import { act, fireEvent, render, screen } from '@testing-library/react';
import { Position, ReactFlow } from '@xyflow/react';
import { MemoryRouter } from 'react-router';
import { TooltipProvider } from '@/components/ui/tooltip';
import { NodeHandleId, Operator } from '../../constant';
import { AgentInstanceContext } from '../../context';
import { useAddNode } from '../../hooks/use-add-node';
import useGraphStore from '../../store';
import { DropdownProvider } from '../context';
import { CommonHandle } from './handle';

// This file uses jest.mock, so it is transformed by esbuild-jest's babel
// pass: imported bindings must not be used in type positions (type-only
// imports break the pass), but annotations over global types are fine.

jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
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

beforeAll(() => {
  // React Flow measures nodes with ResizeObserver and parses zoom transforms
  // with DOMMatrixReadOnly; jsdom has neither (xyflow testing guide recipe).
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  if (!window.ResizeObserver) {
    window.ResizeObserver = ResizeObserverStub;
  }

  class DOMMatrixReadOnlyStub {
    m22: number;
    constructor(transform?: string) {
      const scale = transform?.match(/scale\(([\d.]+)\)/)?.[1];
      this.m22 = scale ? Number(scale) : 1;
    }
  }
  if (!window.DOMMatrixReadOnly) {
    window.DOMMatrixReadOnly =
      DOMMatrixReadOnlyStub as unknown as typeof DOMMatrixReadOnly;
  }
});

function SourceNode({ id }: { id: string }) {
  return (
    <div>
      source
      <CommonHandle
        id="end_cpn_ids"
        type="source"
        position={Position.Right}
        nodeId={id}
        isConnectable
      />
    </div>
  );
}

function DummyGroupNode() {
  return <div>group node</div>;
}

const nodeTypes = {
  sourceNode: SourceNode,
  iterationNode: DummyGroupNode,
  iterationStartNode: DummyGroupNode,
};

function Canvas() {
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);
  const onNodesChange = useGraphStore((state) => state.onNodesChange);
  const onEdgesChange = useGraphStore((state) => state.onEdgesChange);
  const { addCanvasNode } = useAddNode();

  return (
    <AgentInstanceContext.Provider
      value={{ addCanvasNode, showFormDrawer: () => {} }}
    >
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
      />
    </AgentInstanceContext.Provider>
  );
}

const createSwitchNode = () => ({
  id: 'switch:0',
  type: 'sourceNode',
  position: { x: 0, y: 0 },
  data: { label: Operator.Switch, name: 'switch_0', form: {} },
});

function openMenuAndPickIteration() {
  const handle = document.querySelector(
    '.react-flow__handle[data-handleid="end_cpn_ids"]',
  );
  expect(handle).not.toBeNull();
  fireEvent.click(handle as Element);

  const item = screen.getByText('flow.iteration');
  fireEvent.click(item);
}

describe('re-adding a group node from a handle menu after deletion', () => {
  beforeEach(() => {
    useGraphStore.setState({ nodes: [createSwitchNode()], edges: [] });
  });

  it('adds an iteration node twice from the same else handle', () => {
    render(
      <MemoryRouter>
        <TooltipProvider>
          <DropdownProvider>
            <Canvas />
          </DropdownProvider>
        </TooltipProvider>
      </MemoryRouter>,
    );

    openMenuAndPickIteration();

    const firstIteration = useGraphStore
      .getState()
      .nodes.find((x) => x.data.label === Operator.Iteration);
    expect(firstIteration).toBeTruthy();
    expect(useGraphStore.getState().edges).toHaveLength(1);

    act(() => {
      useGraphStore.getState().deleteIterationNodeById(firstIteration!.id);
    });
    expect(
      useGraphStore
        .getState()
        .nodes.some((x) => x.data.label === Operator.Iteration),
    ).toBe(false);
    expect(useGraphStore.getState().edges).toHaveLength(0);

    openMenuAndPickIteration();

    const { nodes, edges } = useGraphStore.getState();
    const secondIteration = nodes.find(
      (x) => x.data.label === Operator.Iteration,
    );
    expect(secondIteration).toBeTruthy();
    expect(
      nodes.some(
        (x) =>
          x.data.label === Operator.IterationStart &&
          x.parentId === secondIteration!.id,
      ),
    ).toBe(true);
    expect(edges).toHaveLength(1);
    expect(edges[0]).toMatchObject({
      source: 'switch:0',
      target: secondIteration!.id,
      sourceHandle: 'end_cpn_ids',
      targetHandle: NodeHandleId.End,
    });
  });
});
