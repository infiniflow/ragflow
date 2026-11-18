import { act, renderHook, waitFor } from '@testing-library/react';
import jsPreviewExcel from '@js-preview/excel';
import axios from 'axios';
import { applyExcelSourceLocate, useFetchExcel } from './hooks';

jest.mock('@js-preview/excel', () => ({
  __esModule: true,
  default: { init: jest.fn() },
}));
jest.mock('ahooks', () => ({
  useDebounceFn: (fn: () => void) => ({
    run: require('react').useCallback(fn, [fn]),
  }),
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
    styles: [] as Record<string, unknown>[],
    rows: {
      setCell: jest.fn(),
      getCellOrNew: jest.fn<{ style?: number }, [number, number]>(() => ({})),
      getHeight: jest.fn(() => 20),
      len: 100,
    },
    cols: { len: 10 },
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

  it('paints every range while scrolling to the first position', () => {
    const previewer = mockPreviewer();
    applyExcelSourceLocate(previewer as never, [
      [1, 20, 20, 1, 1],
      [1, 1, 1, 1, 1],
    ]);
    expect(previewer.xs.datas[0].rows.setCell).toHaveBeenCalledWith(
      19,
      0,
      { style: 0 },
      'format',
    );
    expect(previewer.xs.datas[0].rows.setCell).toHaveBeenCalledWith(
      0,
      0,
      { style: 0 },
      'format',
    );
    expect(previewer.xs.sheet.verticalScrollbar.move).toHaveBeenCalledWith({
      top: 379,
    });
  });

  it('paints ranges on multiple sheets and opens the target sheet', () => {
    const previewer = mockPreviewer();
    applyExcelSourceLocate(previewer as never, [
      [2, 4, 4, 1, 1],
      [1, 2, 2, 1, 1],
    ]);
    expect(previewer.xs.datas[1].rows.setCell).toHaveBeenCalledWith(
      3,
      0,
      { style: 0 },
      'format',
    );
    expect(previewer.xs.datas[0].rows.setCell).toHaveBeenCalledWith(
      1,
      0,
      { style: 0 },
      'format',
    );
    expect(previewer.xs.bottombar.clickSwap2).toHaveBeenCalledWith(
      previewer.xs.bottombar.items[1],
    );
  });

  it('selects the 1-based sheet, paints the range, and scrolls via the scrollbar', () => {
    const previewer = mockPreviewer();
    applyExcelSourceLocate(previewer as never, [[2, 3, 4, 1, 2]]);

    expect(previewer.xs.bottombar.clickSwap2).toHaveBeenCalledWith(
      previewer.xs.bottombar.items[1],
    );
    expect(previewer.xs.datas[1].styles[0]).toEqual({
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
      { style: 0 },
      'format',
    );
    expect(previewer.xs.datas[1].rows.setCell).toHaveBeenCalledWith(
      3,
      1,
      { style: 0 },
      'format',
    );
  });

  it('is a no-op when the position tuple is short', () => {
    const previewer = mockPreviewer();
    applyExcelSourceLocate(previewer as never, [[1, 2]]);
    expect(previewer.xs.datas[0].styles).toHaveLength(0);
  });

  it('skips a range outside the sheet when choosing the scroll target', () => {
    const previewer = mockPreviewer();
    applyExcelSourceLocate(previewer as never, [
      [1, 101, 101, 1, 1],
      [2, 3, 3, 1, 1],
    ]);
    expect(previewer.xs.datas[0].rows.setCell).not.toHaveBeenCalled();
    expect(previewer.xs.bottombar.clickSwap2).toHaveBeenCalledWith(
      previewer.xs.bottombar.items[1],
    );
    expect(previewer.xs.sheet.verticalScrollbar.move).toHaveBeenCalledWith({
      top: 39,
    });
  });

  it('keeps cell formatting and reuses a highlight style for identical styles', () => {
    const previewer = mockPreviewer();
    const sheet = previewer.xs.datas[0];
    const original = {
      font: { bold: true },
      align: 'center',
      border: { bottom: ['thin', '#000000'] },
    };
    Object.assign(sheet, { styles: [original, { ...original }] });
    sheet.rows.getCellOrNew.mockImplementation((row: number) => ({
      style: row % 2,
    }));
    applyExcelSourceLocate(previewer as never, [[1, 1, 2, 1, 1]]);
    const styles = (sheet as unknown as { styles: Record<string, unknown>[] })
      .styles;
    expect(styles).toHaveLength(3);
    expect(styles[2]).toEqual({
      ...original,
      bgcolor: '#ffe58f',
      color: '#1a1a1a',
    });
    expect(sheet.rows.setCell).toHaveBeenCalledWith(
      0,
      0,
      { style: 2 },
      'format',
    );
    expect(sheet.rows.setCell).toHaveBeenCalledWith(
      1,
      0,
      { style: 2 },
      'format',
    );
  });

  it('ignores invalid position entries while highlighting valid ranges', () => {
    const previewer = mockPreviewer();
    expect(() =>
      applyExcelSourceLocate(
        previewer as never,
        [[1, 2, 2, 1, 1], null] as never,
      ),
    ).not.toThrow();
    expect(previewer.xs.datas[0].rows.setCell).toHaveBeenCalled();
  });

  it('does not add more styles when the same cells are highlighted twice', () => {
    const previewer = mockPreviewer();
    const sheet = previewer.xs.datas[0];
    const cells = [{ style: 0 }, { style: 0 }];
    sheet.styles.push({ font: { bold: true } });
    sheet.rows.getCellOrNew.mockImplementation((row: number) => cells[row]);
    sheet.rows.setCell.mockImplementation((row, _col, cell) => {
      cells[row].style = cell.style;
    });
    applyExcelSourceLocate(previewer as never, [[1, 1, 2, 1, 1]]);
    applyExcelSourceLocate(previewer as never, [[1, 1, 2, 1, 1]]);
    expect(sheet.styles).toHaveLength(2);
    expect(sheet.styles[1]).toEqual({
      font: { bold: true },
      bgcolor: '#ffe58f',
      color: '#1a1a1a',
    });
  });

  it('falls back to resetData when clickSwap2 is missing', () => {
    const previewer = mockPreviewer();
    (previewer.xs.bottombar as { clickSwap2?: unknown }).clickSwap2 = undefined;
    applyExcelSourceLocate(previewer as never, [[1, 2, 2, 1, 1]]);
    expect(previewer.xs.sheet.resetData).toHaveBeenCalledWith(
      previewer.xs.datas[0],
    );
  });

  it('sets data.scroll directly when the scrollbar API is unavailable', () => {
    const previewer = mockPreviewer();
    const sheet = previewer.xs.sheet as Record<string, unknown>;
    sheet.verticalScrollbar = undefined;
    sheet.horizontalScrollbar = undefined;
    applyExcelSourceLocate(previewer as never, [[1, 3, 3, 1, 1]]);

    // r1 = 2 rows above, each 20px high
    expect(previewer.xs.datas[0].scroll).toEqual({ x: 0, y: 40 });
    expect(previewer.xs.reRender).toHaveBeenCalled();
    expect(previewer.xs.sheet.table.render).toHaveBeenCalled();
  });
});

describe('useFetchExcel', () => {
  it('updates the preview when only a later position changes', async () => {
    jest.mocked(axios.get).mockResolvedValue({ data: new ArrayBuffer(0) });
    const previews: ReturnType<typeof mockPreviewer>[] = [];
    jest.mocked(jsPreviewExcel.init).mockImplementation(() => {
      const preview = {
        ...mockPreviewer(),
        preview: jest.fn().mockResolvedValue(undefined),
        destroy: jest.fn(),
      };
      previews.push(preview);
      return preview as never;
    });
    const raf = jest
      .spyOn(window, 'requestAnimationFrame')
      .mockImplementation((fn) => {
        fn(0);
        return 1;
      });
    const { result, rerender, unmount } = renderHook(
      ({ positions }) => useFetchExcel('/file.xlsx', positions),
      {
        initialProps: {
          positions: [
            [1, 2, 2, 1, 1],
            [1, 3, 3, 1, 1],
          ],
        },
      },
    );
    act(() => result.current.containerRef(document.createElement('div')));
    await waitFor(() =>
      expect(previews[0]?.xs.datas[0].rows.setCell).toHaveBeenCalledTimes(2),
    );
    rerender({
      positions: [
        [1, 2, 2, 1, 1],
        [1, 4, 4, 1, 1],
      ],
    });
    await waitFor(() =>
      expect(previews[1]?.xs.datas[0].rows.setCell).toHaveBeenCalledWith(
        3,
        0,
        { style: 0 },
        'format',
      ),
    );
    unmount();
    raf.mockRestore();
  });
});
