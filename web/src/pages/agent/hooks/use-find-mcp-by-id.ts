import { useListMcpServer } from '@/hooks/use-mcp-request';

export function useFindMcpById(mcpIds: string[] = []) {
  const { data } = useListMcpServer(mcpIds);

  const findMcpById = (id: string) =>
    data.mcp_servers.find((item) => item.id === id);

  return {
    findMcpById,
  };
}
