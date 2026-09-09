import { pickByBackend } from '@/utils/backend-variant';
import { DataSourceKey } from './constant';

const PythonDataSourceKeys = Object.values(DataSourceKey);
const GoDataSourceKeys = PythonDataSourceKeys.filter(
  (source) => source !== DataSourceKey.FEISHU_WIKI,
);

export const getAvailableDataSourceKeys = (): DataSourceKey[] =>
  pickByBackend({
    go: GoDataSourceKeys,
    python: PythonDataSourceKeys,
  });
