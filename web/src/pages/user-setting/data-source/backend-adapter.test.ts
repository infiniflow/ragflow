(globalThis as any).CSS = {
  ...(globalThis as any).CSS,
  supports: () => false,
};

const loadAvailableDataSourceKeys = () => {
  const { getAvailableDataSourceKeys } = require('./backend-adapter');
  return getAvailableDataSourceKeys();
};

const loadCategorizedDataSourceKeys = () => {
  const { getCategorizedDataSourceKeys } = require('./backend-adapter');
  return getCategorizedDataSourceKeys;
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

  it('registers every built-in source in exactly one category', () => {
    const { DataSourceCategoryGroups, DataSourceKey } = require('./constant');
    const groupedSources = DataSourceCategoryGroups.flatMap(
      (group: (typeof DataSourceCategoryGroups)[number]) => group.sources,
    );
    const sourceKeys = Object.values(DataSourceKey);

    expect(groupedSources).toHaveLength(sourceKeys.length);
    expect(new Set(groupedSources).size).toBe(sourceKeys.length);
    expect(groupedSources.sort()).toEqual(sourceKeys.sort());
  });

  it('categorizes every available source without omissions or duplicates', () => {
    const groups = loadCategorizedDataSourceKeys()();
    const groupedSources = groups.flatMap(
      (group: (typeof groups)[number]) => group.sources,
    );

    expect(groupedSources).toHaveLength(loadAvailableDataSourceKeys().length);
    expect(new Set(groupedSources).size).toBe(groupedSources.length);
    expect(groupedSources.sort()).toEqual(loadAvailableDataSourceKeys().sort());
    expect(
      groups.every(
        (group: (typeof groups)[number]) => group.sources.length > 0,
      ),
    ).toBe(true);
  });

  it('omits empty categories and groups unregistered sources as other', () => {
    const { DataSourceCategory, DataSourceKey } = require('./constant');
    const customSource = 'enterprise_custom';

    expect(
      loadCategorizedDataSourceKeys()([DataSourceKey.S3, customSource]),
    ).toEqual([
      {
        category: DataSourceCategory.ObjectStorage,
        sources: [DataSourceKey.S3],
      },
      {
        category: DataSourceCategory.Other,
        sources: [customSource],
      },
    ]);
  });
});
