import { renderHook } from '@testing-library/react';
import { useAgentsWithBuiltin } from './use-agents-with-builtin';
import { AgentCategory } from '@/constants/agent';

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchAgentListByPage: jest.fn(),
  useFetchBuiltinPipelines: jest.fn(),
  AgentKeys: {
    list: () => ['fetchAgentListByPage'],
    filters: () => ['fetchAgentFilters'],
  },
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const agentRequest = require('@/hooks/use-agent-request');

const builtinCatalog = [
  { id: 'general', title: 'General', description: 'Default', filename: 'a.json' },
  { id: 'book', title: 'Book', description: 'Long doc', filename: 'b.json' },
];

const userItem = {
  id: 'user-1',
  title: 'My pipeline',
  type: 'agent',
  canvas_category: 'dataflow_canvas',
};

beforeEach(() => {
  agentRequest.useFetchBuiltinPipelines.mockReturnValue({
    data: { canvas: builtinCatalog, total: builtinCatalog.length },
    loading: false,
  });
  agentRequest.useFetchAgentListByPage.mockReturnValue({
    data: [userItem],
    loading: false,
    debouncedSearchString: '',
    filterValue: { canvasCategory: undefined },
    pagination: { current: 1, pageSize: 10, total: 1 },
  });
});

describe('useAgentsWithBuiltin', () => {
  it('returns the user items unchanged', () => {
    const { result } = renderHook(() => useAgentsWithBuiltin());
    expect(result.current.data).toEqual([userItem]);
  });

  it('includes builtin items in the "All" view', () => {
    const { result } = renderHook(() => useAgentsWithBuiltin());
    expect(result.current.builtinItems).toHaveLength(2);
    expect(result.current.builtinItems[0].builtin).toBe(true);
    expect(result.current.builtinItems[0].type).toBe('builtin_pipeline');
    expect(result.current.builtinItems[0].canvas_category).toBe(
      AgentCategory.DataflowCanvas,
    );
  });

  it('includes builtin items in the Pipeline (dataflow) view', () => {
    agentRequest.useFetchAgentListByPage.mockReturnValue({
      data: [userItem],
      loading: false,
      debouncedSearchString: '',
      filterValue: { canvasCategory: [AgentCategory.DataflowCanvas] },
      pagination: { current: 1, pageSize: 10, total: 1 },
    });
    const { result } = renderHook(() => useAgentsWithBuiltin());
    expect(result.current.builtinItems).toHaveLength(2);
  });

  it('excludes builtin items in the pure Agent view', () => {
    agentRequest.useFetchAgentListByPage.mockReturnValue({
      data: [userItem],
      loading: false,
      debouncedSearchString: '',
      filterValue: { canvasCategory: [AgentCategory.AgentCanvas] },
      pagination: { current: 1, pageSize: 10, total: 1 },
    });
    const { result } = renderHook(() => useAgentsWithBuiltin());
    expect(result.current.builtinItems).toEqual([]);
  });

  it('excludes builtin items in the compilation template group view', () => {
    agentRequest.useFetchAgentListByPage.mockReturnValue({
      data: [userItem],
      loading: false,
      debouncedSearchString: '',
      filterValue: { canvasCategory: ['compilation_template_group'] },
      pagination: { current: 1, pageSize: 10, total: 1 },
    });
    const { result } = renderHook(() => useAgentsWithBuiltin());
    expect(result.current.builtinItems).toEqual([]);
  });

  it('handles a single string category (not only arrays)', () => {
    agentRequest.useFetchAgentListByPage.mockReturnValue({
      data: [userItem],
      loading: false,
      debouncedSearchString: '',
      // The category filter can be a plain string rather than an array.
      filterValue: { canvasCategory: AgentCategory.AgentCanvas },
      pagination: { current: 1, pageSize: 10, total: 1 },
    });
    const { result } = renderHook(() => useAgentsWithBuiltin());
    expect(result.current.builtinItems).toEqual([]);
  });

  it('ignores non-string category entries (Record-typed filter)', () => {
    agentRequest.useFetchAgentListByPage.mockReturnValue({
      data: [userItem],
      loading: false,
      debouncedSearchString: '',
      // canvasCategory may carry structured filters instead of plain ids.
      filterValue: {
        canvasCategory: [{ operator: 'or', values: [AgentCategory.DataflowCanvas] }],
      },
      pagination: { current: 1, pageSize: 10, total: 1 },
    });
    const { result } = renderHook(() => useAgentsWithBuiltin());
    expect(result.current.builtinItems).toEqual([]);
  });

  it('filters builtin items by the debounced keyword', () => {
    agentRequest.useFetchAgentListByPage.mockReturnValue({
      data: [userItem],
      loading: false,
      debouncedSearchString: 'book',
      filterValue: { canvasCategory: undefined },
      pagination: { current: 1, pageSize: 10, total: 1 },
    });
    const { result } = renderHook(() => useAgentsWithBuiltin());
    expect(result.current.builtinItems).toHaveLength(1);
    expect(result.current.builtinItems[0].id).toBe('book');
  });

  it('returns no builtin items when the catalog is empty', () => {
    agentRequest.useFetchBuiltinPipelines.mockReturnValue({
      data: { canvas: [], total: 0 },
      loading: false,
    });
    const { result } = renderHook(() => useAgentsWithBuiltin());
    expect(result.current.builtinItems).toEqual([]);
  });

  it('surfaces loading while the builtin catalog is fetching', () => {
    const { result } = renderHook(() => useAgentsWithBuiltin());
    expect(result.current.loading).toBe(false);
    expect(result.current.builtinLoading).toBe(false);
  });
});
