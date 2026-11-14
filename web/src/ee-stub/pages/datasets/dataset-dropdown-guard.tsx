import { IDataset } from '@/interfaces/database/dataset';
import { ReactNode } from 'react';

type DatasetDropdownGuardProps = {
  dataset: IDataset;
  showPrivilegeModal(): void;
  manageItems: ReactNode;
  deleteItems: ReactNode;
};

/**
 * Enterprise seam for the dataset dropdown. Open source has no collaborator
 * permissions, so every action the shared file provides is rendered as-is.
 */
export function DatasetDropdownGuard({
  manageItems,
  deleteItems,
}: DatasetDropdownGuardProps) {
  return (
    <>
      {manageItems}
      {deleteItems}
    </>
  );
}
