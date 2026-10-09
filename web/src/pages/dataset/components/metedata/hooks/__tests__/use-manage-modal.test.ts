// The util we are exercising is a pure object of helpers, but the module it
// lives in transitively imports `@/hooks/use-document-request` which drags in
// react-router's createBrowserRouter. That constructor needs the WHATWG
// Request/Response globals that jsdom does not provide. Mock the chain off so
// the test can import the module under the test environment.
jest.mock('@/hooks/use-document-request', () => ({
  DocumentApiAction: {},
}));

import { util } from '../use-manage-modal';

const baseRow = {
  field: 'category',
  description: 'Document category',
  values: ['legal', 'finance'],
  valueType: 'string' as const,
};

describe('util.tableDataToMetaDataSettingJSON - restrictDefinedValues interaction', () => {
  it('emits an empty enum when the switch is explicitly false', () => {
    const [saved] = util.tableDataToMetaDataSettingJSON([
      { ...baseRow, restrictDefinedValues: false },
    ]);
    expect(saved).toEqual({
      key: 'category',
      type: 'string',
      description: 'Document category',
      enum: [],
    });
  });

  it('preserves values when the switch is explicitly true', () => {
    const [saved] = util.tableDataToMetaDataSettingJSON([
      { ...baseRow, restrictDefinedValues: true },
    ]);
    expect(saved.enum).toEqual(['legal', 'finance']);
  });

  it('preserves values when the switch is omitted (undefined)', () => {
    const [saved] = util.tableDataToMetaDataSettingJSON([baseRow]);
    expect(saved.enum).toEqual(['legal', 'finance']);
  });

  it('emits an empty enum when the switch is false even if values are absent', () => {
    const [saved] = util.tableDataToMetaDataSettingJSON([
      {
        field: 'category',
        description: '',
        values: [],
        valueType: 'string',
        restrictDefinedValues: false,
      },
    ]);
    expect(saved.enum).toEqual([]);
  });

  it('covers the list metadata type with the same switch behavior', () => {
    const row = {
      field: 'tags',
      description: 'Document tags',
      values: ['urgent', 'draft'],
      valueType: 'list' as const,
      restrictDefinedValues: false,
    };
    const [saved] = util.tableDataToMetaDataSettingJSON([row]);
    expect(saved.enum).toEqual([]);
    expect(saved.type).toBe('list');
  });

  it('lowercases the valueType so the saved type matches the reader', () => {
    const [saved] = util.tableDataToMetaDataSettingJSON([
      {
        ...baseRow,
        valueType: 'NUMBER' as unknown as typeof baseRow.valueType,
        restrictDefinedValues: true,
      },
    ]);
    expect(saved.type).toBe('number');
  });
});

describe('util.metaDataSettingJSONToMetaDataTableData - array branch', () => {
  it('derives restrictDefinedValues: false when the saved enum is empty', () => {
    const [row] = util.metaDataSettingJSONToMetaDataTableData([
      {
        key: 'category',
        type: 'string',
        description: 'Document category',
        enum: [],
      },
    ]);
    expect(row.restrictDefinedValues).toBe(false);
    expect(row.values).toEqual([]);
  });

  it('derives restrictDefinedValues: true when the saved enum is non-empty', () => {
    const [row] = util.metaDataSettingJSONToMetaDataTableData([
      {
        key: 'category',
        type: 'string',
        description: 'Document category',
        enum: ['legal', 'finance'],
      },
    ]);
    expect(row.restrictDefinedValues).toBe(true);
    expect(row.values).toEqual(['legal', 'finance']);
  });

  it('reads enum out of properties.<key>.items.enum on the JSON-schema branch', () => {
    const [row] = util.metaDataSettingJSONToMetaDataTableData({
      type: 'object',
      properties: {
        category: {
          type: 'string',
          description: 'Document category',
          items: { type: 'string', enum: ['legal', 'finance'] },
        },
      },
    });
    expect(row.restrictDefinedValues).toBe(true);
    expect(row.values).toEqual(['legal', 'finance']);
  });

  it('returns an empty list for an unknown schema branch when no enum is present', () => {
    const [row] = util.metaDataSettingJSONToMetaDataTableData({
      type: 'object',
      properties: {
        category: { type: 'string', description: 'Document category' },
      },
    });
    expect(row.restrictDefinedValues).toBe(false);
    expect(row.values).toEqual([]);
  });
});

describe('util round-trip - explicit OFF survives a save and reopen', () => {
  it('returns restrictDefinedValues: false for a row toggled OFF, then saved, then read back', () => {
    const rows = [{ ...baseRow, restrictDefinedValues: false }];
    const saved = util.tableDataToMetaDataSettingJSON(rows);
    const [reopened] = util.metaDataSettingJSONToMetaDataTableData(saved);
    expect(reopened.restrictDefinedValues).toBe(false);
    expect(reopened.values).toEqual([]);
    expect(reopened.field).toBe('category');
    expect(reopened.description).toBe('Document category');
  });

  it('returns restrictDefinedValues: true when the switch is left ON across the round trip', () => {
    const rows = [{ ...baseRow, restrictDefinedValues: true }];
    const saved = util.tableDataToMetaDataSettingJSON(rows);
    const [reopened] = util.metaDataSettingJSONToMetaDataTableData(saved);
    expect(reopened.restrictDefinedValues).toBe(true);
    expect(reopened.values).toEqual(['legal', 'finance']);
  });

  it('returns restrictDefinedValues: true when the switch is omitted across the round trip', () => {
    const rows = [{ ...baseRow }];
    const saved = util.tableDataToMetaDataSettingJSON(rows);
    const [reopened] = util.metaDataSettingJSONToMetaDataTableData(saved);
    expect(reopened.restrictDefinedValues).toBe(true);
    expect(reopened.values).toEqual(['legal', 'finance']);
  });
});
