import type { TFunction } from 'i18next';
import type { ReactNode } from 'react';

/**
 * Enterprise seam for data-source connectors that are not part of the public
 * product.
 *
 * The data-source page derives every connector from the maps in
 * `pages/user-setting/data-source/constant`, so an enterprise connector only
 * has to contribute its fragments here: the key, the card info, the form
 * fields, the default connection values and its feature switches. Open source
 * ships empty fragments.
 */

export interface EeDataSourceInfoItem {
  name: string;
  description: string;
  icon: ReactNode;
}

/** Connector keys that exist only in the enterprise build. */
export const eeDataSourceKeys: string[] = [];

/** Backend connector source keys controlled by role-level integration permissions. */
export const eeDataSourcePermissionKeys: string[] = [];

/** Connector cards shown in the data-source grid. */
export const generateEeDataSourceInfo = (
  _t: TFunction,
): Record<string, EeDataSourceInfoItem> => ({});

/** Form fields rendered by the add-source dialog. */
export const generateEeDataSourceFormFields = (
  _t: TFunction,
): Record<string, any[]> => ({});

/** Default connection values used to seed the add-source form. */
export const eeDataSourceDefaultValues: Record<
  string,
  Record<string, any>
> = {};

/** Feature switches contributed by enterprise connectors. */
export const EeDataSourceFeatureVisibility: Record<string, any> = {};

/** Fields the detail page must lock once a connector has been created. */
export const shouldDisableEeDataSourceField = (_fieldName: string): boolean =>
  false;

/** Maps the filled form to the backend source the connector uses. */
export const resolveEeDataSourceSource = (values: any): string | undefined =>
  values?.source;
