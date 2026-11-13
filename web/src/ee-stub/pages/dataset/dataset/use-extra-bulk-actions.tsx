import { ShowPrivilegeModalReturnType } from '@/ee/pages/dataset/dataset/use-show-privilege-dialog';
import { ReactNode } from 'react';

export type BulkOperationAction = {
  id: string;
  label: string;
  icon: ReactNode;
  onClick?: () => void;
};

/**
 * Enterprise seam for dataset bulk-operation entries. Open source has no
 * collaborator permissions, so no extra entry is appended.
 */
export function useExtraBulkActions(
  _handShowPrivilegeModal: ShowPrivilegeModalReturnType['handShowPrivilegeModal'],
  _selectedRowKeys: string[],
): BulkOperationAction[] {
  void _handShowPrivilegeModal;
  void _selectedRowKeys;
  return [];
}
