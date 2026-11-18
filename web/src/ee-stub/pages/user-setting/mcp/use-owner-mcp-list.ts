import { IMcpServer } from '@/interfaces/database/mcp';

/**
 * Enterprise seam for the MCP bulk-selection scope. Open source has no
 * collaborator permissions, so every listed server can be selected.
 */
export function useOwnerMcpList(mcpList: IMcpServer[]) {
  return mcpList;
}
