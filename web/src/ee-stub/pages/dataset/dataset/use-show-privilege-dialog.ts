import { IDocumentInfo } from '@/interfaces/database/document';
import { createContext } from 'react';

/**
 * Enterprise seam for the document permission dialog. Open source has no
 * collaborator permissions, so the modal stays closed and the record is empty.
 */
export function useShowPrivilegeDialog() {
  return {
    privilegeModal: false,
    hidePrivilegeModal: () => {},
    handShowPrivilegeModal: (_item: IDocumentInfo | string[]) => () => {},
    recordWithSourceType: {},
  };
}

export type ShowPrivilegeModalReturnType = ReturnType<
  typeof useShowPrivilegeDialog
>;

export const ShowPrivilegeModalContext = createContext<
  Partial<ShowPrivilegeModalReturnType>
>({});
