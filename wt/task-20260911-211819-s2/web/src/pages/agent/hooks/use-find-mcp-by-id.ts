import { useListMcpServer } from '@/hooks/use-mcp-request';

export function useFindMcpById() {
  const { data } = useListMcpServer();

  const findMcpById = (id: string) =>
    data.mcp_servers.find((item) => item.id === id);

  return {
    findMcpById,
  };
}
