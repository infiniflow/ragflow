import { renderHook } from '@testing-library/react';
import { useListMcpServer } from '@/hooks/use-mcp-request';
import { useFindMcpById } from './use-find-mcp-by-id';

jest.mock('@/hooks/use-mcp-request', () => ({
  useListMcpServer: jest.fn(),
}));

const mockUseListMcpServer = jest.mocked(useListMcpServer);

describe('useFindMcpById', () => {
  it('requests selected MCP ids and resolves shared metadata by id', () => {
    mockUseListMcpServer.mockReturnValue({
      data: {
        total: 1,
        mcp_servers: [{ id: 'shared-mcp', name: 'Navigo3' }],
      },
    } as never);

    const { result } = renderHook(() => useFindMcpById(['shared-mcp']));

    expect(mockUseListMcpServer).toHaveBeenCalledWith(['shared-mcp']);
    expect(result.current.findMcpById('shared-mcp')?.name).toBe('Navigo3');
  });
});
