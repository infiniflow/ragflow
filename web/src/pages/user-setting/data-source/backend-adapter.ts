import { DataSourceKey } from './constant';

export const getAvailableDataSourceKeys = (): DataSourceKey[] =>
  Object.values(DataSourceKey);
