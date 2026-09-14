import { renderHook } from '@testing-library/react';
import { Operator } from './constant';
import { useIsPipeline } from './hooks/use-is-pipeline';
import useGraphStore from './store';
import { useValidateConnection } from './hooks';

jest.mock('./hooks/use-is-pipeline', () => ({
  useIsPipeline: jest.fn(),
}));

const mockedUseIsPipeline = jest.mocked(useIsPipeline);

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

const connection = (source: string, target: string) => ({
  source,
  target,
  sourceHandle: null,
  targetHandle: null,
});

describe('useValidateConnection (pipeline linear chain)', () => {
  beforeEach(() => {
    useGraphStore.setState({
      nodes: [
        createNode('compiler:0', Operator.Compiler),
        createNode('tokenizer:0', Operator.Tokenizer),
        createNode('titleChunker:0', Operator.TitleChunker),
        createNode('placeholder:0', Operator.Placeholder),
      ],
      edges: [],
    });
  });

  it('rejects a second downstream on the pipeline canvas', () => {
    mockedUseIsPipeline.mockReturnValue(true);
    useGraphStore.setState({
      edges: [createEdge('e1', 'compiler:0', 'tokenizer:0')],
    });

    const { result } = renderHook(() => useValidateConnection());

    expect(result.current(connection('compiler:0', 'titleChunker:0'))).toBe(
      false,
    );
  });

  it('rejects a second upstream on the pipeline canvas', () => {
    mockedUseIsPipeline.mockReturnValue(true);
    useGraphStore.setState({
      edges: [createEdge('e1', 'compiler:0', 'tokenizer:0')],
    });

    const { result } = renderHook(() => useValidateConnection());

    expect(result.current(connection('titleChunker:0', 'tokenizer:0'))).toBe(
      false,
    );
  });

  it('allows a downstream edge while the source only feeds a pending placeholder', () => {
    mockedUseIsPipeline.mockReturnValue(true);
    useGraphStore.setState({
      edges: [createEdge('e1', 'compiler:0', 'placeholder:0')],
    });

    const { result } = renderHook(() => useValidateConnection());

    expect(result.current(connection('compiler:0', 'titleChunker:0'))).toBe(
      true,
    );
  });

  it('allows branching on the workflow canvas', () => {
    mockedUseIsPipeline.mockReturnValue(false);
    useGraphStore.setState({
      edges: [createEdge('e1', 'compiler:0', 'tokenizer:0')],
    });

    const { result } = renderHook(() => useValidateConnection());

    expect(result.current(connection('compiler:0', 'titleChunker:0'))).toBe(
      true,
    );
  });
});
