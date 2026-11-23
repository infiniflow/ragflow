import { IDataset } from '@/interfaces/database/dataset';

/**
 * Enterprise seam for the dataset permission dialog. Open source has no
 * collaborator permissions, so the modal stays closed and the record is empty.
 */
export function useShowPrivilegeDialog() {
  return {
    privilegeModal: false,
    hidePrivilegeModal: () => {},
    handShowPrivilegeModal: (_item: IDataset) => () => {},
    recordWithSourceType: {},
  };
}

export type ShowPrivilegeDialogReturnType = ReturnType<
  typeof useShowPrivilegeDialog
>;
