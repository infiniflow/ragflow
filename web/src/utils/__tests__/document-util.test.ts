import { buildChunkHighlights, firstHighlightPerPage } from '../document-util';

const SIZE = { width: 849, height: 1200 };

describe('buildChunkHighlights', () => {
  it('returns [] when positions is missing', () => {
    expect(buildChunkHighlights({} as any, SIZE)).toEqual([]);
  });

  it('returns [] when positions is not a list of lists', () => {
    expect(buildChunkHighlights({ positions: [1, 2, 3] as any } as any, SIZE)).toEqual(
      [],
    );
  });

  it('emits one IHighlight per [page, x1, x2, y1, y2] entry', () => {
    const chunk = {
      content_with_weight: 'cell 1 cell 2',
      positions: [
        [85, 100, 200, 300, 400],
        [86, 110, 210, 310, 410],
      ],
    } as any;
    const highlights = buildChunkHighlights(chunk, SIZE);
    expect(highlights).toHaveLength(2);
    expect(highlights[0].position.pageNumber).toBe(85);
    expect(highlights[0].position.boundingRect).toMatchObject({
      x1: 100,
      x2: 200,
      y1: 300,
      y2: 400,
      width: 849,
      height: 1200,
    });
    expect(highlights[1].position.pageNumber).toBe(86);
    expect(highlights[1].position.boundingRect).toMatchObject({
      x1: 110,
      x2: 210,
      y1: 310,
      y2: 410,
    });
  });
});

describe('firstHighlightPerPage', () => {
  it('keeps the first highlight of each distinct page', () => {
    const highlights = [
      { position: { pageNumber: 85 } },
      { position: { pageNumber: 86 } },
      { position: { pageNumber: 85 } }, // duplicate page
      { position: { pageNumber: 86 } },
      { position: { pageNumber: 87 } },
    ] as any;
    const result = firstHighlightPerPage(highlights);
    expect(result.map((h: any) => h.position.pageNumber)).toEqual([85, 86, 87]);
  });

  it('returns the input unchanged for a single-page chunk', () => {
    const highlights = [
      { position: { pageNumber: 1 } },
      { position: { pageNumber: 1 } },
    ] as any;
    expect(firstHighlightPerPage(highlights)).toEqual([
      { position: { pageNumber: 1 } },
    ]);
  });

  it('returns [] for an empty list', () => {
    expect(firstHighlightPerPage([])).toEqual([]);
  });
});
