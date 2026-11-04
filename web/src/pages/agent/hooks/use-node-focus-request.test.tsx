import { act, renderHook } from '@testing-library/react';
import useGraphStore from '../store';
import { useNodeFocusRequest } from './use-node-focus-request';

const createNode = (id: string, label: string) =>
  ({
    id,
    type: 'ragNode',
    position: { x: 0, y: 0 },
    data: { label, name: id, form: {} },
  }) as any;

describe('useNodeFocusRequest', () => {
  beforeEach(() => {
    useGraphStore.setState({
      nodes: [createNode('Message:m1', 'Message')],
      edges: [],
      nodeFocusRequest: null,
      clickedNodeId: '',
      clickedToolId: '',
    });
  });

  it('selects the node, opens its form and centers the viewport on it', () => {
    const showFormDrawerById = jest.fn();
    const fitView = jest.fn();
    renderHook(() =>
      useNodeFocusRequest({
        reactFlowInstance: { fitView },
        showFormDrawerById,
      }),
    );

    act(() => {
      useGraphStore
        .getState()
        .requestNodeFocus('Message:m1', { toolId: 'tool-1' });
    });

    const node = useGraphStore
      .getState()
      .nodes.find((x) => x.id === 'Message:m1');
    expect(node?.selected).toBe(true);
    expect(showFormDrawerById).toHaveBeenCalledWith('Message:m1', 'tool-1');
    expect(fitView).toHaveBeenCalledWith({
      nodes: [{ id: 'Message:m1' }],
      duration: 300,
      maxZoom: 1,
    });
    expect(useGraphStore.getState().nodeFocusRequest).toBeNull();
  });

  it('only highlights the node when openForm is false (orphan issues)', () => {
    const showFormDrawerById = jest.fn();
    renderHook(() => useNodeFocusRequest({ showFormDrawerById }));

    act(() => {
      useGraphStore
        .getState()
        .requestNodeFocus('Message:m1', { openForm: false });
    });

    expect(useGraphStore.getState().nodes[0].selected).toBe(true);
    expect(showFormDrawerById).not.toHaveBeenCalled();
    expect(useGraphStore.getState().nodeFocusRequest).toBeNull();
  });

  it('clears requests for nodes that no longer exist', () => {
    const showFormDrawerById = jest.fn();
    renderHook(() => useNodeFocusRequest({ showFormDrawerById }));

    act(() => {
      useGraphStore.getState().requestNodeFocus('Message:gone');
    });

    expect(showFormDrawerById).not.toHaveBeenCalled();
    expect(useGraphStore.getState().nodeFocusRequest).toBeNull();
  });

  it('refires when the same node is focused twice in a row', () => {
    const showFormDrawerById = jest.fn();
    renderHook(() => useNodeFocusRequest({ showFormDrawerById }));

    act(() => {
      useGraphStore.getState().requestNodeFocus('Message:m1');
    });
    act(() => {
      useGraphStore.getState().requestNodeFocus('Message:m1');
    });

    expect(showFormDrawerById).toHaveBeenCalledTimes(2);
  });
});
