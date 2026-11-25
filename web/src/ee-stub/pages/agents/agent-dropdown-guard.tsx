import { IFlow } from '@/interfaces/database/agent';
import { ReactNode } from 'react';

type AgentDropdownGuardProps = {
  agent: IFlow;
  showPrivilegeModal(): void;
  manageItems: ReactNode;
  deleteItems: ReactNode;
};

export function shouldShowAgentDropdown(_agent: IFlow) {
  return true;
}

/**
 * Enterprise seam for the agent dropdown. Open source has no collaborator
 * permissions, so every action the shared file provides is rendered as-is.
 */
export function AgentDropdownGuard({
  manageItems,
  deleteItems,
}: AgentDropdownGuardProps) {
  return (
    <>
      {manageItems}
      {deleteItems}
    </>
  );
}
