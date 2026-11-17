import { IMcpServer } from '@/interfaces/database/mcp';
import { ReactNode } from 'react';

type McpSelectionGuardProps = {
  mcp: IMcpServer;
  children: ReactNode;
};

/**
 * Enterprise seam for the MCP card selection control. Open source has no
 * collaborator permissions, so every listed server is selectable.
 */
export function McpSelectionGuard({ children }: McpSelectionGuardProps) {
  return <>{children}</>;
}
