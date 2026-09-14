(globalThis as any).CSS = {
  ...(globalThis as any).CSS,
  supports: () => false,
};

const loadAvailableDataSourceKeys = () => {
  const { getAvailableDataSourceKeys } = require('./backend-adapter');
  return getAvailableDataSourceKeys();
};

describe('data source backend adapter', () => {
  it('includes Feishu Wiki so the connector is available on both backends', () => {
    expect(loadAvailableDataSourceKeys()).toContain('feishu_wiki');
  });

  it('returns every DataSourceKey', () => {
    const { DataSourceKey } = require('./constant');
    expect(loadAvailableDataSourceKeys().sort()).toEqual(
      Object.values(DataSourceKey).sort(),
    );
  });
});
