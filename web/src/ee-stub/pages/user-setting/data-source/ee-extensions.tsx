import type { TFunction } from 'i18next';
import type { ReactNode } from 'react';

/**
 * Enterprise seam for data-source connectors that are not part of the public
 * product.
 *
 * The data-source page derives every connector from the maps in
 * `pages/user-setting/data-source/constant`, so an enterprise connector only
 * has to contribute its fragments here: the key, the card info, the form
 * fields and the default connection values. Open source ships empty
 * fragments.
 */

export interface EeDataSourceInfoItem {
  name: string;
  description: string;
  icon: ReactNode;
}

/** Connector keys that exist only in the enterprise build. */
export const eeDataSourceKeys: string[] = [];

/** Connector cards shown in the data-source grid. */
export const generateEeDataSourceInfo = (
  _t: TFunction,
): Record<string, EeDataSourceInfoItem> => ({});

/** Form fields rendered by the add-source dialog. */
export const eeDataSourceFormFields: Record<string, any[]> = {};

/** Default connection values used to seed the add-source form. */
export const eeDataSourceDefaultValues: Record<
  string,
  Record<string, any>
> = {};
