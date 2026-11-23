import type { ReactNode } from 'react';

export type DatasetSidebarExtraItem = {
  icon: ReactNode;
  label: string;
  key: string;
};

/**
 * Enterprise seam for the dataset sidebar. Open source keeps no version
 * history, so the menu contributes no extra item here.
 */
export function useDatasetSidebarExtraItems(): DatasetSidebarExtraItem[] {
  return [];
}
