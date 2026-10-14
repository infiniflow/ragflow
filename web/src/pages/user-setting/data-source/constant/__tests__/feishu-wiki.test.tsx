(globalThis as any).CSS = {
  ...(globalThis as any).CSS,
  supports: () => false,
};

const { FormFieldType } = require('@/components/dynamic-form');
const {
  DataSourceFormDefaultValues,
  DataSourceKey,
  generateDataSourceInfo,
  getDataSourceFieldsWithExtras,
} = require('../index');

const translate = ((key: string) => key) as any;

describe('Feishu Wiki data source form', () => {
  it('registers the Python-backed file source and its generic description', () => {
    const key = (DataSourceKey as any).FEISHU_WIKI;

    expect(key).toBe('feishu_wiki');
    expect(generateDataSourceInfo(translate)[key]).toMatchObject({
      name: 'Feishu Wiki',
      description: 'setting.feishu_wikiDescription',
    });
  });

  it('exposes credentials, pre-download filters, and bounded resource controls', () => {
    const key = (DataSourceKey as any).FEISHU_WIKI;
    const fields = getDataSourceFieldsWithExtras(translate, key);
    const names = fields.map((field: { name: string }) => field.name);

    expect(names).toEqual(
      expect.arrayContaining([
        'config.credentials.app_id',
        'config.credentials.app_secret',
        'config.space_id',
        'config.root_node_token',
        'config.include_extensions',
        'config.include_keywords',
        'config.exclude_keywords',
        'config.max_file_size_bytes',
        'config.batch_size',
      ]),
    );
    expect(
      fields.find(
        (field: { name: string; type: string }) =>
          field.name === 'config.credentials.app_secret',
      )?.type,
    ).toBe(FormFieldType.Password);
    expect(
      fields.find(
        (field: { name: string; type: string }) =>
          field.name === 'config.include_extensions',
      )?.type,
    ).toBe(FormFieldType.Tag);
    expect(
      fields.find(
        (field: {
          name: string;
          validation?: { min?: number; max?: number };
        }) => field.name === 'config.batch_size',
      )?.validation,
    ).toMatchObject({ min: 1, max: 10 });
    expect(
      fields.find(
        (field: { name: string; validation?: { min?: number } }) =>
          field.name === 'config.max_file_size_bytes',
      )?.validation,
    ).toMatchObject({ min: 1 });
  });

  it('defaults to empty filters, a 50 MiB file budget, and batch size two', () => {
    const key = (DataSourceKey as any).FEISHU_WIKI;
    const defaults = (DataSourceFormDefaultValues as any)[key];

    expect(defaults).toEqual({
      name: '',
      source: 'feishu_wiki',
      config: {
        space_id: '',
        root_node_token: '',
        include_extensions: [],
        include_keywords: [],
        exclude_keywords: [],
        max_file_size_bytes: 50 * 1024 * 1024,
        batch_size: 2,
        credentials: {
          app_id: '',
          app_secret: '',
        },
      },
    });
  });
});
