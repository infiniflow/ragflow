import { act, renderHook } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { useWatchFormChange } from './use-watch-change';

const mockUpdateNodeForm = jest.fn();
const mockTools = { first: { name: 'first' }, second: { name: 'second' } };
const mockAgent = {
  id: 'agent:1',
  mcp: [{ mcp_id: 'mcp:1', tools: mockTools }],
};
const mockFindUpstream = () => mockAgent;
let mockResponse: any;

jest.mock('@/hooks/use-mcp-request', () => ({
  useGetMcpServer: () => mockResponse,
}));
jest.mock('@/pages/agent/store', () => ({
  __esModule: true,
  default: (selector: (state: any) => unknown) =>
    selector({
      clickedToolId: 'mcp:1',
      clickedNodeId: 'tool:1',
      findUpstreamNodeById: mockFindUpstream,
      updateNodeForm: mockUpdateNodeForm,
    }),
}));
jest.mock('@/pages/agent/utils', () => ({
  getAgentNodeMCP: (node: typeof mockAgent) => node.mcp,
}));

function useWatcher() {
  const form = useForm({ defaultValues: { items: ['first', 'second'] } });
  useWatchFormChange(form);
  return form;
}

describe('MCP form persistence', () => {
  beforeEach(() => {
    mockUpdateNodeForm.mockClear();
    mockResponse = {
      data: { id: 'mcp:1', variables: { tools: mockTools } },
      loading: false,
    };
  });

  it('does not write on opening the panel', () => {
    renderHook(useWatcher);
    expect(mockUpdateNodeForm).not.toHaveBeenCalled();
  });

  it.each([
    { data: {}, loading: true },
    { data: {}, loading: false },
    { data: { id: 'other', variables: { tools: mockTools } }, loading: false },
    { data: { id: 'mcp:1', variables: { tools: {} } }, loading: false },
  ])(
    'preserves tools when the response is unavailable or incomplete: %j',
    (response) => {
      mockResponse = response;
      const { result } = renderHook(useWatcher);
      act(() =>
        result.current.setValue('items', ['first'], { shouldDirty: true }),
      );
      expect(mockUpdateNodeForm).not.toHaveBeenCalled();
    },
  );

  it('preserves configured tools for a dirty empty selection and an empty catalog', () => {
    mockResponse = {
      data: { id: 'mcp:1', variables: { tools: {} } },
      loading: false,
    };
    const { result } = renderHook(useWatcher);
    act(() => result.current.setValue('items', [], { shouldDirty: true }));
    expect(mockUpdateNodeForm).not.toHaveBeenCalled();
    expect(mockAgent.mcp[0].tools).toEqual(mockTools);
  });

  it('persists an explicit valid selection', () => {
    const { result } = renderHook(useWatcher);
    act(() =>
      result.current.setValue('items', ['first'], { shouldDirty: true }),
    );
    expect(mockUpdateNodeForm).toHaveBeenLastCalledWith(
      'agent:1',
      [{ mcp_id: 'mcp:1', tools: { first: mockTools.first } }],
      ['mcp'],
    );
  });

  it('allows explicitly deselecting all tools after a successful fetch', () => {
    const { result } = renderHook(useWatcher);
    act(() => result.current.setValue('items', [], { shouldDirty: true }));
    expect(mockUpdateNodeForm).toHaveBeenLastCalledWith(
      'agent:1',
      [{ mcp_id: 'mcp:1', tools: {} }],
      ['mcp'],
    );
  });

  it('persists a user restoring the initial selection after an edit', () => {
    const { result } = renderHook(useWatcher);
    act(() =>
      result.current.setValue('items', ['first'], { shouldDirty: true }),
    );
    const originalMCP = mockAgent.mcp;
    mockAgent.mcp = mockUpdateNodeForm.mock.calls.at(-1)[1];
    mockUpdateNodeForm.mockClear();
    try {
      act(() =>
        result.current.setValue('items', ['first', 'second'], {
          shouldDirty: true,
        }),
      );
      expect(mockUpdateNodeForm).toHaveBeenLastCalledWith(
        'agent:1',
        originalMCP,
        ['mcp'],
      );
    } finally {
      mockAgent.mcp = originalMCP;
    }
  });

  it('does not replace a valid selection when the next fetch fails', () => {
    const { result, rerender } = renderHook(useWatcher);
    act(() =>
      result.current.setValue('items', ['first'], { shouldDirty: true }),
    );
    mockUpdateNodeForm.mockClear();
    mockResponse = { data: {}, loading: false };
    rerender();
    expect(mockUpdateNodeForm).not.toHaveBeenCalled();
  });
});
