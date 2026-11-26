import { IMemory } from '@/pages/memories/interface';
import { ReactNode } from 'react';

type MemoryDropdownGuardProps = {
  memory: IMemory;
  showPrivilegeModal(): void;
  manageItems: ReactNode;
  deleteItems: ReactNode;
};

export function shouldShowMemoryDropdown(_memory: IMemory) {
  return true;
}

export function canManageMemoryConfig(_operatorPermission = 0) {
  return true;
}

export function canWriteMemory(_operatorPermission = 0) {
  return true;
}

export function useCanCreateMemory() {
  return true;
}

export function useCanWriteMemoryRole(_tenantId: string) {
  return true;
}

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
