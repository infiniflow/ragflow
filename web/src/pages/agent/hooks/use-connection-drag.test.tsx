import { act, renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { DropdownProvider } from '../canvas/context';
import { Operator } from '../constant';
import useGraphStore from '../store';
import { useConnectionDrag } from './use-connection-drag';

jest.mock('./use-is-pipeline', () => ({
  useIsPipeline: jest.fn(() => true),
}));

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchDefaultModelDictionary: () => ({}),
}));

const noop = () => {};

const createNode = (id: string, label: string) => ({
  id,
  type: 'ragNode',
  position: { x: 0, y: 0 },
  data: { label, name: id, form: {} },
});

const createEdge = (id: string, source: string, target: string) => ({
  id,
  source,
  target,
});

function renderDragHook() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderHook(
    () =>
      useConnectionDrag(
        noop,
        noop,
        noop,
        noop,
        noop,
        (x, y) => ({ x, y }),
        noop,
        noop,
        noop,
      ),
    {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>
          <MemoryRouter>
            <DropdownProvider>{children}</DropdownProvider>
          </MemoryRouter>
        </QueryClientProvider>
      ),
    },
  );
}

const dragFrom = (
  hook: ReturnType<typeof renderDragHook>,
  nodeId = 'compiler:0',
) => {
  act(() => {
    (hook.result.current.onConnectStart as any)(
      { clientX: 100, clientY: 100 },
      { nodeId, handleId: 'start' },
    );
    (hook.result.current.onConnectEnd as any)({
      clientX: 400,
      clientY: 400,
    });
  });
};

const hasPlaceholder = () =>
  useGraphStore
    .getState()
    .nodes.some((node) => node.data?.label === Operator.Placeholder);

describe('useConnectionDrag (pipeline canvas)', () => {
  beforeEach(() => {
    useGraphStore.setState({ nodes: [], edges: [] });
  });

  it('keeps the drag origin available to the next-step dropdown after the drag ends', () => {
    useGraphStore.setState({
      nodes: [createNode('compiler:0', Operator.Compiler)],
      edges: [],
    });

    const hook = renderDragHook();
    dragFrom(hook);

    expect(hook.result.current.nodeId).toBe('compiler:0');
    expect(hook.result.current.getConnectionStartContext()).toMatchObject({
      nodeId: 'compiler:0',
      id: 'start',
      type: 'source',
      isFromConnectionDrag: true,
    });
    expect(hasPlaceholder()).toBe(true);
  });

  it('does not open a next-step branch when the node already has a downstream', () => {
    useGraphStore.setState({
      nodes: [
        createNode('compiler:0', Operator.Compiler),
        createNode('tokenizer:0', Operator.Tokenizer),
      ],
      edges: [createEdge('e1', 'compiler:0', 'tokenizer:0')],
    });

    const hook = renderDragHook();
    dragFrom(hook);

    expect(hasPlaceholder()).toBe(false);
    expect(hook.result.current.nodeId).toBeUndefined();
    expect(hook.result.current.getConnectionStartContext()).toBeNull();
  });

  it('still opens the next step while the only downstream is a pending placeholder', () => {
    useGraphStore.setState({
      nodes: [
        createNode('compiler:0', Operator.Compiler),
        createNode('placeholder:0', Operator.Placeholder),
      ],
      edges: [createEdge('e1', 'compiler:0', 'placeholder:0')],
    });

    const hook = renderDragHook();
    dragFrom(hook);

    expect(hook.result.current.nodeId).toBe('compiler:0');
  });

  it('treats a press without movement as a handle click', () => {
    useGraphStore.setState({
      nodes: [createNode('compiler:0', Operator.Compiler)],
      edges: [],
    });

    const hook = renderDragHook();
    act(() => {
      (hook.result.current.onConnectStart as any)(
        { clientX: 100, clientY: 100 },
        { nodeId: 'compiler:0', handleId: 'start' },
      );
      (hook.result.current.onConnectEnd as any)({
        clientX: 101,
        clientY: 101,
      });
    });

    expect(hasPlaceholder()).toBe(false);
    expect(hook.result.current.nodeId).toBeUndefined();
  });

  it('clears the pending origin when the canvas moves', () => {
    useGraphStore.setState({
      nodes: [createNode('compiler:0', Operator.Compiler)],
      edges: [],
    });

    const hook = renderDragHook();
    dragFrom(hook);

    act(() => {
      hook.result.current.onMove();
      hook.rerender();
    });

    expect(hook.result.current.nodeId).toBeUndefined();
    expect(hook.result.current.getConnectionStartContext()).toBeNull();
  });
});
