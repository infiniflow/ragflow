import { IDialog } from '@/interfaces/database/chat';
import { ReactNode } from 'react';

type ChatDropdownGuardProps = {
  chat: IDialog;
  showPrivilegeModal(): void;
  manageItems: ReactNode;
  deleteItems: ReactNode;
};

/**
 * Enterprise seam for the chat dropdown. Open source has no collaborator
 * permissions, so every action the shared file provides is rendered as-is.
 */
export function ChatDropdownGuard({
  manageItems,
  deleteItems,
}: ChatDropdownGuardProps) {
  return (
    <>
      {manageItems}
      {deleteItems}
    </>
  );
}
