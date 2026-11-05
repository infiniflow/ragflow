import { util } from '../use-manage-modal';

jest.mock('@/components/ui/message', () => ({}));
jest.mock('@/hooks/common-hooks', () => ({}));
jest.mock('@/hooks/logic-hooks/use-row-selection', () => ({}));
jest.mock('@/hooks/use-document-request', () => ({}));
jest.mock('@/services/knowledge-service', () => ({}));

describe('metadata restriction settings', () => {
  it.each(['string', 'list'] as const)(
    'keeps a disabled %s restriction off after saving and reopening',
    (type) => {
      const [row] = util.metaDataSettingJSONToMetaDataTableData([
        { key: 'category', type, enum: ['legal', 'finance'] },
      ]);
      row.restrictDefinedValues = false;

      const saved = util.tableDataToMetaDataSettingJSON([row]);
      const [reopened] = util.metaDataSettingJSONToMetaDataTableData(saved);

      expect(saved).toEqual([
        expect.objectContaining({ key: 'category', type, enum: [] }),
      ]);
      expect(reopened.restrictDefinedValues).toBe(false);
      expect(row.values).toEqual(['legal', 'finance']);
    },
  );

  it('removes a restriction imported from a JSON schema item enum', () => {
    const [row] = util.metaDataSettingJSONToMetaDataTableData({
      properties: {
        categories: { type: 'list', items: { enum: ['legal', 'finance'] } },
      },
    });
    expect(row.restrictDefinedValues).toBe(true);

    const saved = util.tableDataToMetaDataSettingJSON([
      { ...row, restrictDefinedValues: false },
    ]);

    expect(util.metaDataSettingJSONToMetaDataTableData(saved)[0]).toMatchObject(
      {
        values: [],
        restrictDefinedValues: false,
      },
    );
  });

  it.each([
    ['string', true],
    ['string', undefined],
    ['list', true],
    ['list', undefined],
  ] as const)(
    'preserves %s allowed values when the restriction flag is %s',
    (valueType, restrictDefinedValues) => {
      const row = {
        field: 'category',
        description: 'Document category',
        valueType,
        values: ['legal', 'finance'],
        restrictDefinedValues,
      };
      const saved = util.tableDataToMetaDataSettingJSON([row]);

      expect(util.metaDataSettingJSONToMetaDataTableData(saved)[0]).toEqual({
        ...row,
        restrictDefinedValues: true,
      });
    },
  );

  it.each(['string', 'list', 'number', 'time'] as const)(
    'keeps unrestricted %s fields without enum values',
    (valueType) => {
      const row = {
        field: 'field',
        description: '',
        valueType,
        values: [],
        restrictDefinedValues: false,
      };
      const saved = util.tableDataToMetaDataSettingJSON([row]);

      expect(util.metaDataSettingJSONToMetaDataTableData(saved)[0]).toEqual(
        row,
      );
    },
  );

  it('preserves document values independently of the schema restriction', () => {
    expect(
      util.tableDataToMetaDataJSON([
        {
          field: 'category',
          description: '',
          values: ['legal', 'finance'],
          restrictDefinedValues: false,
        },
      ]),
    ).toEqual({ category: ['legal', 'finance'] });
  });
});
