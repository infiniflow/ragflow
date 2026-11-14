import { IDialog } from '@/interfaces/database/chat';

/**
 * Enterprise seam for the chat permission dialog. Open source has no
 * collaborator permissions, so the modal stays closed and the record is empty.
 */
export function useShowPrivilegeDialog() {
  return {
    privilegeModalVisible: false,
    hidePrivilegeModal: () => {},
    privilegeRecord: {},
    handleShowPrivilegeModal: (_dialog: IDialog) => (_info?: any) => {},
  };
}

export type ShowPrivilegeDialogReturnType = ReturnType<
  typeof useShowPrivilegeDialog
>;
