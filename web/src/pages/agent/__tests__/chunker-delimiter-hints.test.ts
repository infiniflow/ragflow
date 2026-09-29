import {
  getChunkerChildrenDelimiterPreview,
  getChunkerDelimiterPreview,
  getChunkerDelimiterTipKey,
} from '../utils';

describe('getChunkerDelimiterTipKey', () => {
  it('points at the delimiter tip', () => {
    expect(getChunkerDelimiterTipKey()).toBe('flow.delimitersTip');
  });
});

describe('getChunkerDelimiterPreview', () => {
  it('previews every non-empty row as one delimiter', () => {
    expect(
      getChunkerDelimiterPreview(['\n', '`##`']).map((d) => d.raw),
    ).toEqual(['##', '\n']);
  });
});

describe('getChunkerChildrenDelimiterPreview', () => {
  it('keeps bare rows', () => {
    expect(
      getChunkerChildrenDelimiterPreview(['\n', '`##`']).map((d) => d.raw),
    ).toEqual(['##', '\n']);
  });
});
