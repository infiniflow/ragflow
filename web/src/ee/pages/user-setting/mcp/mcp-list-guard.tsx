import { IMcpServer } from '@/interfaces/database/mcp';
import { ReactNode } from 'react';

type McpToolbarGuardProps = {
  mcpList: IMcpServer[];
  bulkManageButton: ReactNode;
  children: ReactNode;
};

type McpSelectionBarGuardProps = {
  mcpList: IMcpServer[];
  children: ReactNode;
};

type McpEmptyStateGuardProps = {
  mcpList: IMcpServer[];
  addButton: ReactNode;
  onClickAdd(): void;
  children: ReactNode;
};

/**
 * Enterprise seams for the MCP server list. Open source has no collaborator
 * permissions, so the toolbar, the bulk-selection bar and the empty state are
 * always available with every action.
 */
export function McpToolbarGuard({
  bulkManageButton,
  children,
}: McpToolbarGuardProps) {
  return (
    <>
      {bulkManageButton}
      {children}
    </>
  );
}

export function McpSelectionBarGuard({ children }: McpSelectionBarGuardProps) {
  return <>{children}</>;
}

export function McpEmptyStateGuard({
  addButton,
  onClickAdd,
  children,
}: McpEmptyStateGuardProps) {
  return (
    <div
      className="flex items-center justify-between border border-dashed border-border-button rounded-md p-10 cursor-pointer w-[590px]"
      onClick={onClickAdd}
    >
      {children}
      {addButton}
    </div>
  );
}
