import { useListMcpServer } from '@/hooks/use-mcp-request';
import { useResourceTenantId } from '../context';

export function useFindMcpById() {
  const resourceTenantId = useResourceTenantId();
  const { data } = useListMcpServer(
    resourceTenantId,
    Boolean(resourceTenantId),
  );

  const findMcpById = (id: string) =>
    data.mcp_servers.find((item) => item.id === id);

  return {
    findMcpById,
  };
}
