import { IMcpServer } from '@/interfaces/database/mcp';

/**
 * Enterprise seam for the MCP server permission dialog. Open source has no
 * collaborator permissions, so the modal stays closed and the record is empty.
 */
export function useShowPrivilegeDialog() {
  return {
    privilegeModal: false,
    hidePrivilegeModal: () => {},
    handShowPrivilegeModal: (_item: IMcpServer) => () => {},
    recordWithSourceType: {},
  };
}

export type ShowPrivilegeDialogReturnType = ReturnType<
  typeof useShowPrivilegeDialog
>;
