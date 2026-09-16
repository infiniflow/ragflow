import request from '@/utils/request';
import { extractTableColumns, isTableFile } from '../table-column-extract';

jest.mock('@/utils/request', () => ({
  post: jest.fn(),
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

  describe('extractTableColumns', () => {
    it('returns columns from backend probe when available', async () => {
      (request.post as unknown as jest.Mock).mockResolvedValueOnce({
        data: {
          code: 0,
          data: {
            columns: ['Name', 'City', 'Role'],
            total_columns: 3,
          },
        },
      });

      const file = new File(['Name,City,Role\n'], 'test.csv', {
        type: 'text/csv',
      });
      const columns = await extractTableColumns(file);

      expect(request.post).toHaveBeenCalledTimes(1);
      expect(columns).toEqual(['Name', 'City', 'Role']);
    });

    it('falls back to local parsing if server probe errors', async () => {
      (request.post as unknown as jest.Mock).mockRejectedValueOnce(
        new Error('Network error'),
      );

      const csvContent = 'Name,Age,Name\nAlice,30,Bob\n';
      const file = new File([csvContent], 'test.csv', { type: 'text/csv' });
      const columns = await extractTableColumns(file);

      expect(columns).toEqual(['Name', 'Age', 'Name_2']);
    });

    it('falls back to local TSV parsing with tab delimiter', async () => {
      (request.post as unknown as jest.Mock).mockRejectedValueOnce(
        new Error('Probe disabled'),
      );

      const tsvContent = 'ID\tProduct\tPrice\n1\tWidget\t10\n';
      const file = new File([tsvContent], 'test.tsv', {
        type: 'text/tab-separated-values',
      });
      const columns = await extractTableColumns(file);

      expect(columns).toEqual(['ID', 'Product', 'Price']);
    });
  });
});
