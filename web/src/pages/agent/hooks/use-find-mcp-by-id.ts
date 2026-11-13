import { useListMcpServer } from '@/hooks/use-mcp-request';
import { useOwnerTenantId } from '../context';

export function useFindMcpById(mcpIds: string[] = []) {
  const ownerTenantId = useOwnerTenantId();
  const { data } = useListMcpServer(
    ownerTenantId,
    Boolean(ownerTenantId) || mcpIds.length > 0,
    mcpIds,
  );

  const findMcpById = (id: string) =>
    data.mcp_servers.find((item) => item.id === id);

  return {
    findMcpById,
  };
}
