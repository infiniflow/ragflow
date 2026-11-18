import { IFlow } from '@/interfaces/database/agent';

/**
 * Enterprise seam for the agent permission dialog. Open source has no
 * collaborator permissions, so the modal stays closed and the record is empty.
 */
export function useShowPrivilegeDialog() {
  return {
    privilegeModalVisible: false,
    hidePrivilegeModal: () => {},
    handShowPrivilegeModal: (_item: IFlow) => () => {},
    recordWithSourceType: {},
  };
}

export type ShowPrivilegeDialogReturnType = ReturnType<
  typeof useShowPrivilegeDialog
>;
