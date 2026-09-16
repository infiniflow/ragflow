import { buildTableUploadParserConfig } from './table-upload-parser-config';

const csvFile = (name = 'data.csv') =>
  new File(['a,b\n1,2'], name, { type: 'text/csv' });
const xlsxFile = (name = 'book.xlsx') => new File(['x'], name);
const tsvFile = (name = 'data.tsv') => new File(['a\tb'], name);
const pdfFile = (name = 'report.pdf') =>
  new File(['%PDF-1.4'], name, { type: 'application/pdf' });

const baseValues = {
  tableColumnMode: 'manual' as const,
  tableColumnNames: ['a', 'b', 'c'],
  tableColumnNamesByFile: [] as string[][],
  tableColumnRoles: { a: 'indexing' as const },
};

describe('buildTableUploadParserConfig', () => {
  // Regression: the per-file entries used to be compacted to table files only
  // (tableIndexes.filter(...)), so the backend — which pairs entry [i] with the
  // i-th uploaded file — wrote each file's columns onto the previous document.
  it('keeps table_column_names_by_file aligned with the full file list', () => {
    const files = [pdfFile(), csvFile(), xlsxFile()];
    // The upload dialog emits one entry per selected file, empty for non-table files.
    const tableColumnNamesByFile = [[], ['a', 'b'], ['c']];

    const config = buildTableUploadParserConfig(files, {
      ...baseValues,
      tableColumnNamesByFile,
    });

    expect(config).toEqual({
      table_column_mode: 'manual',
      table_column_names: ['a', 'b', 'c'],
      table_column_names_by_file: [[], ['a', 'b'], ['c']],
      table_column_roles: { a: 'indexing' },
    });
    expect(config?.table_column_names_by_file[1]).toEqual(['a', 'b']);
    expect(config?.table_column_names_by_file[2]).toEqual(['c']);
  });

  it('pads a short per-file list so later files do not shift', () => {
    const config = buildTableUploadParserConfig([csvFile(), xlsxFile()], {
      ...baseValues,
      tableColumnNamesByFile: [['a', 'b']],
    });

    expect(config?.table_column_names_by_file).toEqual([['a', 'b'], []]);
  });

  it('treats tsv as a table file', () => {
    const config = buildTableUploadParserConfig([tsvFile()], {
      ...baseValues,
      tableColumnNamesByFile: [['a', 'b']],
    });

    expect(config?.table_column_mode).toBe('manual');
    expect(config?.table_column_names_by_file).toEqual([['a', 'b']]);
  });

  it('sends nothing when the upload has no table file', () => {
    const config = buildTableUploadParserConfig([pdfFile()], {
      ...baseValues,
      tableColumnNamesByFile: [['a']],
    });

    expect(config).toBeUndefined();
  });

  it('omits per-file and role keys when only the mode is set', () => {
    const config = buildTableUploadParserConfig([csvFile()], {
      tableColumnMode: 'auto',
      tableColumnNames: [],
      tableColumnNamesByFile: [[], []],
      tableColumnRoles: {},
    });

    expect(config).toEqual({ table_column_mode: 'auto' });
  });
});
