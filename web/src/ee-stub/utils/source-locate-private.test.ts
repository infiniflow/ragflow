import type { IHighlight } from 'react-pdf-highlighter';
import { getSourceLocatePageNumber } from './source-locate-private';

const HighlightAtThirdPage: IHighlight = {
  id: 'highlight-3',
  comment: { text: '', emoji: '' },
  content: { text: 'PDF chunk' },
  position: {
    pageNumber: 3,
    boundingRect: {
      x1: 10,
      x2: 20,
      y1: 15,
      y2: 25,
      width: 72,
      height: 72,
    },
    rects: [],
  },
};

describe('getSourceLocatePageNumber OSS fallback', () => {
  it('always keeps the existing first-page behavior', () => {
    expect(getSourceLocatePageNumber()).toBe(1);
    expect(getSourceLocatePageNumber([])).toBe(1);
    expect(getSourceLocatePageNumber([HighlightAtThirdPage])).toBe(1);
  });
});
