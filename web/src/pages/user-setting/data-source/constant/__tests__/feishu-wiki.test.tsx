(globalThis as any).CSS = {
  ...(globalThis as any).CSS,
  supports: () => false,
};

const { FormFieldType } = require('@/components/dynamic-form');
const {
  DataSourceFormDefaultValues,
  DataSourceKey,
  getDataSourceFieldsWithExtras,
} = require('../index');

const translate = ((key: string) => key) as any;

describe('Feishu Wiki data source form', () => {
  it('exposes credentials and pre-download screening rules', () => {
    const key = (DataSourceKey as any).FEISHU_WIKI;
    expect(key).toBe('feishu_wiki');

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
  });

  it('defaults to non-destructive empty filters and a bounded batch', () => {
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
        batch_size: 2,
        credentials: {
          app_id: '',
          app_secret: '',
        },
      },
    });
  });
});
