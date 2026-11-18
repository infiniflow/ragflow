import { eeDataSourceKeys } from '@/ee/pages/user-setting/data-source/ee-extensions';
import { DataSourceKey } from './constant';

export const getAvailableDataSourceKeys = (): DataSourceKey[] =>
  [...Object.values(DataSourceKey), ...eeDataSourceKeys] as DataSourceKey[];
