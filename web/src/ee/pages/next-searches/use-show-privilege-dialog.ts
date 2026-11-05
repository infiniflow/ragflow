import { ISearchAppProps } from '@/pages/next-searches/hooks';

/**
 * Enterprise seam for the search-app permission dialog. Open source has no
 * collaborator permissions, so the modal stays closed and the record is empty.
 */
export function useShowPrivilegeDialog() {
  return {
    privilegeModalVisible: false,
    hidePrivilegeModal: () => {},
    handShowPrivilegeModal: (_item: ISearchAppProps) => () => {},
    recordWithSourceType: {},
  };
}

export type ShowPrivilegeDialogReturnType = ReturnType<
  typeof useShowPrivilegeDialog
>;
