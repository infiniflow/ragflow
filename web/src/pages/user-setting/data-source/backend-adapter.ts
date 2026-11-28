import { eeDataSourceKeys } from '@/ee/pages/user-setting/data-source/ee-extensions';
import {
  DataSourceCategory,
  DataSourceCategoryGroups,
  DataSourceKey,
} from './constant';

export const getAvailableDataSourceKeys = (): DataSourceKey[] =>
  [...Object.values(DataSourceKey), ...eeDataSourceKeys] as DataSourceKey[];

export const getCategorizedDataSourceKeys = (
  availableKeys: DataSourceKey[] = getAvailableDataSourceKeys(),
) => {
  const registeredKeys = new Set(
    DataSourceCategoryGroups.flatMap((group) => group.sources),
  );
  const availableKeySet = new Set(availableKeys);
  const groups = DataSourceCategoryGroups.map((group) => ({
    category: group.category,
    sources: group.sources.filter((source) => availableKeySet.has(source)),
  })).filter((group) => group.sources.length > 0);
  const otherSources = availableKeys.filter((key) => !registeredKeys.has(key));

  if (otherSources.length > 0) {
    groups.push({ category: DataSourceCategory.Other, sources: otherSources });
  }

  return groups;
};
