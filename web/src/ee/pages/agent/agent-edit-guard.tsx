import { ReactNode } from 'react';

type AgentEditGuardProps = {
  operatorPermission: number;
  children: ReactNode;
};

/**
 * Enterprise seam for the agent canvas editing controls. Open source has no
 * collaborator permissions, so every control stays visible.
 */
export function AgentEditGuard({ children }: AgentEditGuardProps) {
  return <>{children}</>;
}
