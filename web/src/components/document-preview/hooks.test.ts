import { applyExcelSourceLocate } from './hooks';

jest.mock('@js-preview/excel', () => ({
  __esModule: true,
  default: { init: jest.fn() },
}));
jest.mock('ahooks', () => ({
  useDebounceFn: () => ({ run: jest.fn() }),
  useSize: () => ({ width: 800, height: 600 }),
}));
jest.mock('axios', () => ({ get: jest.fn() }));
jest.mock('jszip');
jest.mock('xlsx', () => ({ read: jest.fn(), write: jest.fn() }));
jest.mock('@/hooks/route-hook', () => ({
  useGetKnowledgeSearchParams: () => ({}),
}));
jest.mock('@/pages/dataflow-result/hooks', () => ({
  useGetPipelineResultSearchParams: () => ({}),
}));
jest.mock('@/utils/api', () => ({
  default: {},
  restAPIv1: '',
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: () => '',
}));
jest.mock('@/constants/authorization', () => ({
  Authorization: 'Authorization',
}));

function mockPreviewer(sheetCount = 2) {
  const makeData = () => ({
    addStyle: jest.fn(() => 7),
    rows: {
      setCell: jest.fn(),
      getCellOrNew: jest.fn(),
      getHeight: jest.fn(() => 20),
    },
    scroll: { x: 0, y: 0 },
  });
  const datas = Array.from({ length: sheetCount }, makeData);
  const items = datas.map(() => document.createElement('div'));
  return {
    xs: {
      datas,
      reRender: jest.fn(),
      sheet: {
        resetData: jest.fn(),
        data: datas[0],
        reload: jest.fn(),
        table: { render: jest.fn() },
        verticalScrollbar: { move: jest.fn() },
        horizontalScrollbar: { move: jest.fn() },
      },
      bottombar: {
        items,
        clickSwap2: jest.fn(),
      },
    },
  };
}

describe('applyExcelSourceLocate', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('selects the 1-based sheet, paints the range, and scrolls via the scrollbar', () => {
    const previewer = mockPreviewer();
    applyExcelSourceLocate(previewer as never, [2, 3, 4, 1, 2]);

    expect(previewer.xs.bottombar.clickSwap2).toHaveBeenCalledWith(
      previewer.xs.bottombar.items[1],
    );
    expect(previewer.xs.datas[1].addStyle).toHaveBeenCalledWith({
      bgcolor: '#ffe58f',
      color: '#1a1a1a',
    });
    expect(previewer.xs.sheet.horizontalScrollbar.move).toHaveBeenCalledWith({
      left: 0,
    });
    // r1 = 2 rows above at 20px each -> y = 40, and scrolly() needs y - 1.
    expect(previewer.xs.sheet.verticalScrollbar.move).toHaveBeenCalledWith({
      top: 39,
    });
    expect(previewer.xs.datas[1].rows.setCell).toHaveBeenCalledTimes(4);
    expect(previewer.xs.datas[1].rows.setCell).toHaveBeenCalledWith(
      2,
      0,
      { style: 7 },
      'format',
    );
    expect(previewer.xs.datas[1].rows.setCell).toHaveBeenCalledWith(
      3,
      1,
      { style: 7 },
      'format',
    );
  });

  it('is a no-op when the position tuple is short', () => {
    const previewer = mockPreviewer();
    applyExcelSourceLocate(previewer as never, [1, 2]);
    expect(previewer.xs.datas[0].addStyle).not.toHaveBeenCalled();
  });

  it('falls back to resetData when clickSwap2 is missing', () => {
    const previewer = mockPreviewer();
    (previewer.xs.bottombar as { clickSwap2?: unknown }).clickSwap2 = undefined;
    applyExcelSourceLocate(previewer as never, [1, 2, 2, 1, 1]);
    expect(previewer.xs.sheet.resetData).toHaveBeenCalledWith(
      previewer.xs.datas[0],
    );
  });

  it('sets data.scroll directly when the scrollbar API is unavailable', () => {
    const previewer = mockPreviewer();
    const sheet = previewer.xs.sheet as Record<string, unknown>;
    sheet.verticalScrollbar = undefined;
    sheet.horizontalScrollbar = undefined;
    applyExcelSourceLocate(previewer as never, [1, 3, 3, 1, 1]);

    // r1 = 2 rows above, each 20px high
    expect(previewer.xs.datas[0].scroll).toEqual({ x: 0, y: 40 });
    expect(previewer.xs.reRender).toHaveBeenCalled();
    expect(previewer.xs.sheet.table.render).toHaveBeenCalled();
  });
});
