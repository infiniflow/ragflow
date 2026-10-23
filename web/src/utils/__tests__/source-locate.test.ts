import { supportsSourceLocate } from '../source-locate';

describe('supportsSourceLocate', () => {
  it('accepts every previewable document extension, case-insensitively', () => {
    expect(supportsSourceLocate('pdf')).toBe(true);
    expect(supportsSourceLocate('XLSX')).toBe(true);
    expect(supportsSourceLocate('xls')).toBe(true);
    // Support follows the previewable set rather than a hand-picked list: a
    // document the preview can render is also one the citation can locate in.
    expect(supportsSourceLocate('DOCX')).toBe(true);
  });

  it('rejects extensions outside the previewable set and missing ones', () => {
    // HTML is previewable but not locatable: it renders as one whole document,
    // so there is no offset a citation could point at.
    expect(supportsSourceLocate('HTML')).toBe(false);
    expect(supportsSourceLocate('exe')).toBe(false);
    expect(supportsSourceLocate('bin')).toBe(false);
    expect(supportsSourceLocate(undefined)).toBe(false);
  });
});
