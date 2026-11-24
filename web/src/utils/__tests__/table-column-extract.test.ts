import { extractTableColumns, isTableFile } from '../table-column-extract';

describe('table upload columns', () => {
  it('uses tabs for TSV files with commas in the column names', async () => {
    const file = new File(
      ['question, aliases\tanswer, notes\nwhere, exactly?\tParis, France'],
      'faq.TSV',
    );
    expect(isTableFile(file)).toBe(true);
    await expect(extractTableColumns(file)).resolves.toEqual([
      'question, aliases',
      'answer, notes',
    ]);
  });

  it('still detects the delimiter of a CSV export', async () => {
    const file = new File(['question;answer\nWhere?;Paris'], 'faq.csv');
    await expect(extractTableColumns(file)).resolves.toEqual([
      'question',
      'answer',
    ]);
  });
});
