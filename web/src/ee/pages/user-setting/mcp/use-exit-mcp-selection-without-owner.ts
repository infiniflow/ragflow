import { IMcpServer } from '@/interfaces/database/mcp';

/**
 * Enterprise seam for the MCP bulk-selection availability. Open source has no
 * collaborator permissions, so a selection never has to be exited because the
 * operator lost owner access.
 */
export function useExitMcpSelectionWithoutOwner(
  _mcpList: IMcpServer[],
  _isSelectionMode: boolean,
  _exitSelectionMode: () => void,
) {
  void _mcpList;
  void _isSelectionMode;
  void _exitSelectionMode;
}
