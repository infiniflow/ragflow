import type { ReactNode } from 'react';

export type AdminNavItem = {
  path: string;
  name: string;
  icon: ReactNode;
};

export type AdminNavExtension = {
  items: AdminNavItem[];
  hiddenPaths: string[];
};

const emptyExtension: AdminNavExtension = { items: [], hiddenPaths: [] };

/**
 * Enterprise-only admin navigation. The open-source build adds no entries and
 * hides none; the enterprise build replaces this file with the real entries.
 */
export function useEnterpriseAdminNav(): AdminNavExtension {
  return emptyExtension;
}
