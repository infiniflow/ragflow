import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { listMcpServers } from '@/services/mcp-server-service';
import { useListMcpServer } from '../use-mcp-request';

jest.mock('@/services/mcp-server-service', () => ({
  __esModule: true,
  default: {},
  listMcpServers: jest.fn(),
}));

jest.mock('../logic-hooks', () => ({
  useHandleSearchChange: () => ({
    searchString: 'ignored search',
    setSearchString: jest.fn(),
    handleInputChange: jest.fn(),
  }),
  useGetPaginationWithRouter: () => ({
    pagination: { current: 4, pageSize: 10 },
    setPagination: jest.fn(),
  }),
}));

jest.mock('ahooks', () => ({
  useDebounce: (value: string) => value,
}));

const mockListMcpServers = jest.mocked(listMcpServers);

function Wrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe('useListMcpServer', () => {
  beforeEach(() => {
    mockListMcpServers.mockReset();
    mockListMcpServers.mockResolvedValue({
      data: { data: { total: 0, mcp_servers: [] } },
    } as never);
  });

  it('requests all selected MCP metadata on the first page', async () => {
    renderHook(
      () => useListMcpServer(undefined, true, ['mcp-1', 'mcp-2', 'mcp-3']),
      { wrapper: Wrapper },
    );

    await waitFor(() => expect(mockListMcpServers).toHaveBeenCalled());

    expect(mockListMcpServers).toHaveBeenCalledWith({
      keywords: '',
      page: 1,
      page_size: 3,
      mcp_ids: 'mcp-1,mcp-2,mcp-3',
    });
  });

  it('keeps the owner tenant on selected metadata requests', async () => {
    renderHook(() => useListMcpServer('owner-tenant', true, ['shared-mcp']), {
      wrapper: Wrapper,
    });
    await waitFor(() => expect(mockListMcpServers).toHaveBeenCalled());
    expect(mockListMcpServers).toHaveBeenCalledWith({
      keywords: '',
      page: 1,
      page_size: 1,
      tenant_id: 'owner-tenant',
      mcp_ids: 'shared-mcp',
    });
  });

  it('does not fetch selected metadata when disabled', () => {
    renderHook(() => useListMcpServer('owner-tenant', false, ['shared-mcp']), {
      wrapper: Wrapper,
    });
    expect(mockListMcpServers).not.toHaveBeenCalled();
  });

  it('preserves search pagination for the regular MCP list', async () => {
    renderHook(() => useListMcpServer(), { wrapper: Wrapper });

    await waitFor(() => expect(mockListMcpServers).toHaveBeenCalled());

    expect(mockListMcpServers).toHaveBeenCalledWith({
      keywords: 'ignored search',
      page: 4,
      page_size: 10,
    });
  });
});
