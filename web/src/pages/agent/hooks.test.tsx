import { renderHook } from '@testing-library/react';
import { PropsWithChildren } from 'react';
import { MemoryRouter } from 'react-router';
import { NodeHandleId, Operator } from './constant';
import { useValidateConnection } from './hooks';
import useGraphStore from './store';

const createNode = (id: string, label: Operator) => ({
  id,
  type: 'ragNode',
  position: { x: 0, y: 0 },
  data: {
    label,
    name: id,
    form: {},
  },
});

const wrapper = ({ children }: PropsWithChildren) => (
  <MemoryRouter>{children}</MemoryRouter>
);

describe('useValidateConnection', () => {
  beforeEach(() => {
    useGraphStore.setState({
      nodes: [
        createNode('Agent:a', Operator.Agent),
        createNode('CodeExec:b', Operator.Code),
      ],
      edges: [],
    });
  });

  const renderValidator = () =>
    renderHook(() => useValidateConnection(), { wrapper });

  it('allows a normal start -> end connection', () => {
    const { result } = renderValidator();

    expect(
      result.current({
        source: 'Agent:a',
        sourceHandle: NodeHandleId.Start,
        target: 'CodeExec:b',
        targetHandle: NodeHandleId.End,
      }),
    ).toBe(true);
  });

  // With ConnectionMode.Loose a drag started on a target handle and dropped
  // on another target handle is reported flipped (drop node as source) with
  // the "end" id on both sides; such an edge can never resolve its source
  // handle and renders as a ghost edge.
  it('rejects a flipped target -> target connection', () => {
    const { result } = renderValidator();

    expect(
      result.current({
        source: 'CodeExec:b',
        sourceHandle: NodeHandleId.End,
        target: 'Agent:a',
        targetHandle: NodeHandleId.End,
      }),
    ).toBe(false);
  });

  it('rejects a source -> source connection', () => {
    const { result } = renderValidator();

    expect(
      result.current({
        source: 'Agent:a',
        sourceHandle: NodeHandleId.Start,
        target: 'CodeExec:b',
        targetHandle: NodeHandleId.Start,
      }),
    ).toBe(false);
  });
});
