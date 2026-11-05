import { IMemory } from '@/pages/memories/interface';
import { ReactNode } from 'react';

type MemoryDropdownGuardProps = {
  memory: IMemory;
  showPrivilegeModal(): void;
  manageItems: ReactNode;
  deleteItems: ReactNode;
};

/**
 * Enterprise seam for the memory dropdown. Open source has no collaborator
 * permissions, so every action the shared file provides is rendered as-is.
 */
export function MemoryDropdownGuard({
  manageItems,
  deleteItems,
}: MemoryDropdownGuardProps) {
  return (
    <>
      {manageItems}
      {deleteItems}
    </>
  );
}
