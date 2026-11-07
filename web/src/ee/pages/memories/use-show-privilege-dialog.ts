import { IMemory } from '@/pages/memories/interface';

/**
 * Enterprise seam for the memory permission dialog. Open source has no
 * collaborator permissions, so the modal stays closed and the record is empty.
 */
export function useShowPrivilegeDialog() {
  return {
    privilegeModalVisible: false,
    hidePrivilegeModal: () => {},
    handShowPrivilegeModal: (_item: IMemory) => () => {},
    recordWithSourceType: {},
  };
}

export type ShowPrivilegeDialogReturnType = ReturnType<
  typeof useShowPrivilegeDialog
>;
