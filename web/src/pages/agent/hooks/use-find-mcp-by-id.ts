import { useListMcpServer } from '@/hooks/use-mcp-request';
import { useOwnerTenantId } from '../context';

export function useFindMcpById() {
  const ownerTenantId = useOwnerTenantId();
  const { data } = useListMcpServer(ownerTenantId, Boolean(ownerTenantId));

  const findMcpById = (id: string) =>
    data.mcp_servers.find((item) => item.id === id);

  return {
    findMcpById,
  };
}
