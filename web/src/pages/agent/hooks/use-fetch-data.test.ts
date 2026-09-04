import { act, renderHook } from '@testing-library/react';
import { useFetchDataOnMount } from './use-fetch-data';

const mockSetGraphInfo = jest.fn();
const mockRefetch = jest.fn();
const mockUseFetchAgent = jest.fn();

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchAgent: () => mockUseFetchAgent(),
}));

jest.mock('./use-set-graph', () => ({
  useSetGraphInfo: () => mockSetGraphInfo,
}));

jest.mock('../utils/dsl-bridge', () => ({
  dslToGraph: (dsl: { graph?: { nodes: unknown[]; edges: unknown[] } }) => ({
    nodes: dsl.graph?.nodes ?? [],
    edges: dsl.graph?.edges ?? [],
  }),
}));

describe('useFetchDataOnMount', () => {
  beforeEach(() => {
    mockSetGraphInfo.mockClear();
    mockRefetch.mockClear();
  });

  it('does not apply an empty graph while agent detail has no dsl', () => {
    mockUseFetchAgent.mockReturnValue({
      loading: true,
      data: {},
      refetch: mockRefetch,
    });

    renderHook(() => useFetchDataOnMount());

    expect(mockSetGraphInfo).not.toHaveBeenCalled();
  });

  it('applies the fetched dsl once it is present', () => {
    mockUseFetchAgent.mockReturnValue({
      loading: false,
      data: {
        id: 'agent-1',
        dsl: { graph: { nodes: [{ id: 'begin:0' }], edges: [] } },
      },
      refetch: mockRefetch,
    });

    renderHook(() => useFetchDataOnMount());

    expect(mockSetGraphInfo).toHaveBeenCalledWith({
      nodes: [{ id: 'begin:0' }],
      edges: [],
    });
  });

  it('does not clear a loaded canvas when later detail snapshots omit dsl', () => {
    mockUseFetchAgent.mockReturnValue({
      loading: false,
      data: {
        id: 'agent-1',
        dsl: { graph: { nodes: [{ id: 'begin:0' }], edges: [] } },
      },
      refetch: mockRefetch,
    });

    const { rerender } = renderHook(() => useFetchDataOnMount());

    mockUseFetchAgent.mockReturnValue({
      loading: false,
      data: {},
      refetch: mockRefetch,
    });

    act(() => {
      rerender();
    });

    expect(mockSetGraphInfo).toHaveBeenCalledTimes(1);
  });
});
