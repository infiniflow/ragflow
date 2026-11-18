import { useListMcpServer } from '@/hooks/use-mcp-request';
import { ModelResourceAccessUserIdContext } from '@/hooks/use-llm-request';
import { useContext } from 'react';
import { useResourceTenantId } from '../context';

export function useFindMcpById() {
  const resourceTenantId = useResourceTenantId();
  const accessUserId = useContext(ModelResourceAccessUserIdContext);
  const { data } = useListMcpServer(
    resourceTenantId,
    Boolean(resourceTenantId),
    accessUserId,
  );

  const findMcpById = (id: string) =>
    data.mcp_servers.find((item) => item.id === id);

  return {
    findMcpById,
  };
}
