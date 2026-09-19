import { probeTableColumns } from '@/services/knowledge-service';
import { extractTableColumns, isTableFile } from '../table-column-extract';

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
      expect(isTableFile(new File([], 'test.txt'))).toBe(true);
      expect(isTableFile(new File([], 'test.xlsx'))).toBe(true);
      expect(isTableFile(new File([], 'test.xls'))).toBe(true);
    });

    it('returns false for non-table extensions', () => {
      expect(isTableFile(new File([], 'test.pdf'))).toBe(false);
      expect(isTableFile(new File([], 'test.docx'))).toBe(false);
      expect(isTableFile(new File([], 'test.png'))).toBe(false);
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
