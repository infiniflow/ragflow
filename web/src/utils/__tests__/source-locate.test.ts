import { supportsSourceLocate } from '../source-locate';

describe('supportsSourceLocate', () => {
  it('accepts pdf and excel extensions case-insensitively', () => {
    expect(supportsSourceLocate('pdf')).toBe(true);
    expect(supportsSourceLocate('XLSX')).toBe(true);
    expect(supportsSourceLocate('xls')).toBe(true);
  });

  it('rejects unsupported or missing extensions', () => {
    expect(supportsSourceLocate('html')).toBe(false);
    expect(supportsSourceLocate('docx')).toBe(false);
    expect(supportsSourceLocate(undefined)).toBe(false);
  });
});
