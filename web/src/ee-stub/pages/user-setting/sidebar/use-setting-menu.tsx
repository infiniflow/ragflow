import { ReactNode, useMemo } from 'react';

export type SettingMenuEntry = {
  icon: ReactNode;
  label: string;
  key: string;
  'data-testid'?: string;
};

/**
 * Enterprise seam for the settings sidebar entries. Open source has no billing
 * page and no model-provider exposure rules, so the upstream menu is shown
 * unchanged.
 */
export function useSettingMenuItems<T extends SettingMenuEntry>(
  items: T[],
): T[] {
  return useMemo(() => items, [items]);
}
