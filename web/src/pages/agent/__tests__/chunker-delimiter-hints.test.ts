// Kept out of the co-located utils.test.ts: files containing jest.mock are
// transformed through the babel path (see jest-esbuild-transformer.cjs), which
// cannot handle an imported type used in a type annotation.
import {
  getChunkerChildrenDelimiterPreview,
  getChunkerDelimiterPreview,
  getChunkerDelimiterTipKey,
} from '../utils';

// Dispatch on the mutable flag so each scenario can exercise either the
// Go or the Python branch without mounting React.
let mockIsGoBackend = false;
jest.mock('@/utils/backend-variant', () => ({
  pickByBackend: ({ go, python }: { go: unknown; python: unknown }) =>
    mockIsGoBackend ? go : python,
}));

describe('getChunkerDelimiterTipKey', () => {
  afterEach(() => {
    mockIsGoBackend = false;
  });

  it('points at the Go delimiter tip on the Go backend', () => {
    mockIsGoBackend = true;
    expect(getChunkerDelimiterTipKey()).toBe('flow.delimitersTip');
  });

  it('points at the Python delimiter tip on the Python backend', () => {
    expect(getChunkerDelimiterTipKey()).toBe('flow.delimitersTipPython');
  });
});

describe('getChunkerDelimiterPreview', () => {
  afterEach(() => {
    mockIsGoBackend = false;
  });

  it('previews every non-empty row as one delimiter on the Go backend', () => {
    mockIsGoBackend = true;
    expect(
      getChunkerDelimiterPreview(['\n', '`##`']).map((d) => d.raw),
    ).toEqual(['##', '\n']);
  });

  it('previews only backtick-wrapped rows on the Python backend', () => {
    expect(
      getChunkerDelimiterPreview(['\n', '`##`']).map((d) => d.raw),
    ).toEqual(['##']);
  });
});

describe('getChunkerChildrenDelimiterPreview', () => {
  afterEach(() => {
    mockIsGoBackend = false;
  });

  it('keeps bare rows on both backends', () => {
    expect(
      getChunkerChildrenDelimiterPreview(['\n', '`##`']).map((d) => d.raw),
    ).toEqual(['##', '\n']);

    mockIsGoBackend = true;
    expect(
      getChunkerChildrenDelimiterPreview(['\n', '`##`']).map((d) => d.raw),
    ).toEqual(['##', '\n']);
  });
});
