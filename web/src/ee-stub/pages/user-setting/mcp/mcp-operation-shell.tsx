import { IMcpServer } from '@/interfaces/database/mcp';
import { UseEditMcpReturnType } from '@/pages/user-setting/mcp/use-edit-mcp';
import { PropsWithChildren } from 'react';

export type McpOperationShellProps = PropsWithChildren<
  { mcp: IMcpServer } & Pick<UseEditMcpReturnType, 'showEditModal'> & {
      showPrivilegeModal(): void;
    }
>;

/**
 * Enterprise seam for the MCP server card actions. Open source renders the
 * upstream hover controls unchanged.
 */
export function McpOperationShell({ children }: McpOperationShellProps) {
  return <>{children}</>;
}
