import { ISearchAppProps } from '@/pages/next-searches/hooks';
import { ReactNode } from 'react';

type SearchDropdownGuardProps = {
  dataset: ISearchAppProps;
  showPrivilegeModal(): void;
  manageItems: ReactNode;
  deleteItems: ReactNode;
};

/**
 * Enterprise seam for the search-app dropdown. Open source has no collaborator
 * permissions, so every action the shared file provides is rendered as-is.
 */
export function SearchDropdownGuard({
  manageItems,
  deleteItems,
}: SearchDropdownGuardProps) {
  return (
    <>
      {manageItems}
      {deleteItems}
    </>
  );
}
