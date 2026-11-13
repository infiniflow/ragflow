(globalThis as any).CSS = {
  ...(globalThis as any).CSS,
  supports: () => false,
};

const loadAvailableDataSourceKeys = () => {
  const { getAvailableDataSourceKeys } = require('./backend-adapter');
  return getAvailableDataSourceKeys();
};

describe('data source backend adapter', () => {
  it('returns every DataSourceKey', () => {
    const { DataSourceKey } = require('./constant');
    const {
      eeDataSourceKeys,
    } = require('@/ee/pages/user-setting/data-source/ee-extensions');
    expect(loadAvailableDataSourceKeys().sort()).toEqual(
      [...Object.values(DataSourceKey), ...eeDataSourceKeys].sort(),
    );
  });
});
