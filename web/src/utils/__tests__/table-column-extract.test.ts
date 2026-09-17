import { probeTableColumns } from '@/services/knowledge-service';
import {
  extractTableColumns,
  isTableFile,
  resolveDatasetTableColumnSettings,
} from '../table-column-extract';

jest.mock('@/services/knowledge-service', () => ({
  probeTableColumns: jest.fn(),
}));

describe('table-column-extract', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('isTableFile', () => {
    it('returns true for table file extensions', () => {
      expect(isTableFile(new File([], 'test.csv'))).toBe(true);
      expect(isTableFile(new File([], 'test.tsv'))).toBe(true);
      expect(isTableFile(new File([], 'test.xlsx'))).toBe(true);
      expect(isTableFile(new File([], 'test.xls'))).toBe(true);
    });

    it('returns false for non-table extensions', () => {
      expect(isTableFile(new File([], 'test.pdf'))).toBe(false);
      expect(isTableFile(new File([], 'test.docx'))).toBe(false);
      expect(isTableFile(new File([], 'test.png'))).toBe(false);
    });
  });

  describe('resolveDatasetTableColumnSettings', () => {
    it('prefers root-level dataset settings', () => {
      expect(
        resolveDatasetTableColumnSettings({
          table_column_mode: 'manual',
          table_column_roles: { a: 'vectorize', b: 'nonsense' },
          'Parser:Table': {
            spreadsheet: { column_mode: 'auto', column_roles: { a: 'both' } },
          },
        }),
      ).toEqual({ mode: 'manual', roles: { a: 'indexing', b: 'both' } });
    });

    it('falls back to the component-shaped entry', () => {
      expect(
        resolveDatasetTableColumnSettings({
          'Parser:Table': {
            spreadsheet: {
              column_mode: 'manual',
              column_roles: { a: 'metadata' },
            },
          },
        }),
      ).toEqual({ mode: 'manual', roles: { a: 'metadata' } });
    });

    // The runtime compares both values verbatim: only the exact "manual"
    // selects manual (common.NormalizeTableColumnMode) and a role is matched
    // without trimming or case-folding, so the dialog must not show a choice
    // ingestion will not honour, nor drop a column name that is merely blank.
    it('reads a stored mode and stored roles exactly', () => {
      expect(
        resolveDatasetTableColumnSettings({ table_column_mode: ' Manual ' }),
      ).toEqual({ mode: 'auto', roles: {} });

      expect(
        resolveDatasetTableColumnSettings({
          table_column_roles: { ' Indexing ': ' INDEXING ', '': 'metadata' },
        }),
      ).toEqual({
        mode: 'auto',
        roles: { ' Indexing ': 'both', '': 'metadata' },
      });
    });

    // Any of mode, roles or names makes the root level authoritative, even when
    // the manual profile is on the canvas (indexdoc.ResolveTableProfile).
    it('treats stored column names alone as a root-level setting', () => {
      expect(
        resolveDatasetTableColumnSettings({
          table_column_names: ['a'],
          'Parser:Table': {
            spreadsheet: {
              column_mode: 'manual',
              column_roles: { a: 'metadata' },
            },
          },
        }),
      ).toEqual({ mode: 'auto', roles: {} });
    });

    // A blank name is still a column of the schema, so a list holding only one
    // counts as stated: indexdoc.parseTableColumnNames keeps it verbatim.
    it('counts a blank column name as a stated schema', () => {
      expect(
        resolveDatasetTableColumnSettings({
          table_column_names: [''],
          'Parser:Table': {
            spreadsheet: {
              column_mode: 'manual',
              column_roles: { a: 'metadata' },
            },
          },
        }),
      ).toEqual({ mode: 'auto', roles: {} });
    });

    // A component entry that states nothing is not a profile either: a canvas
    // can carry an empty spreadsheet block, and the resolution has to reach the
    // component that does declare one.
    it('skips a component entry that states nothing', () => {
      expect(
        resolveDatasetTableColumnSettings({
          'Parser:A': { spreadsheet: {} },
          'Parser:B': {
            spreadsheet: {
              column_mode: 'manual',
              column_roles: { a: 'metadata' },
            },
          },
        }),
      ).toEqual({ mode: 'manual', roles: { a: 'metadata' } });
    });

    it('defaults to auto with no settings', () => {
      expect(resolveDatasetTableColumnSettings(undefined)).toEqual({
        mode: 'auto',
        roles: {},
      });
      expect(resolveDatasetTableColumnSettings({})).toEqual({
        mode: 'auto',
        roles: {},
      });
    });
  });

  describe('extractTableColumns', () => {
    it('returns columns from backend probe when available', async () => {
      (probeTableColumns as unknown as jest.Mock).mockResolvedValueOnce({
        code: 0,
        data: {
          columns: ['Name', 'City', 'Role'],
          total_columns: 3,
        },
      });

      const file = new File(['Name,City,Role\n'], 'test.csv', {
        type: 'text/csv',
      });
      const columns = await extractTableColumns(file);

      expect(probeTableColumns).toHaveBeenCalledTimes(1);
      expect(columns).toEqual(['Name', 'City', 'Role']);
    });

    it('falls back to local parsing if server probe errors', async () => {
      (probeTableColumns as unknown as jest.Mock).mockRejectedValueOnce(
        new Error('Network error'),
      );

      const csvContent = 'Name,Age,Name\nAlice,30,Bob\n';
      const file = new File([csvContent], 'test.csv', { type: 'text/csv' });
      const columns = await extractTableColumns(file);

      expect(columns).toEqual(['Name', 'Age', 'Name_2']);
    });

    it('falls back to local TSV parsing with tab delimiter', async () => {
      (probeTableColumns as unknown as jest.Mock).mockRejectedValueOnce(
        new Error('Probe disabled'),
      );

      const tsvContent = 'ID\tProduct\tPrice\n1\tWidget\t10\n';
      const file = new File([tsvContent], 'test.tsv', {
        type: 'text/tab-separated-values',
      });
      const columns = await extractTableColumns(file);

      expect(columns).toEqual(['ID', 'Product', 'Price']);
    });

    it('falls back to a tab delimiter for a .txt table too', async () => {
      (probeTableColumns as unknown as jest.Mock).mockRejectedValueOnce(
        new Error('Probe disabled'),
      );

      // rag/app/table.py splits a .txt table on a tab, so the comma inside the
      // first field is part of the column name.
      const txtContent = 'name,full\tamount\nAlice,Ann\t10\n';
      const file = new File([txtContent], 'test.txt', { type: 'text/plain' });
      const columns = await extractTableColumns(file);

      expect(columns).toEqual(['name,full', 'amount']);
    });

    it('takes a delimited header exactly as ingestion indexes it', async () => {
      (probeTableColumns as unknown as jest.Mock).mockRejectedValueOnce(
        new Error('Probe disabled'),
      );

      // Only a spreadsheet trims a header cell and names an empty one by
      // position; a delimited column keeps the spelling the file gives it, so
      // the fallback must not offer a trimmed name or an invented Column_2.
      const csvContent = ' Name,,amount\nAlice,1,10\n';
      const file = new File([csvContent], 'test.csv', { type: 'text/csv' });
      const columns = await extractTableColumns(file);

      expect(columns).toEqual([' Name', '', 'amount']);
    });
  });
});
