import jsPreviewExcel from '@js-preview/excel';
import * as XLSX from 'xlsx';
import { applyExcelSourceLocate } from './hooks';

// Exercise the bundled previewer and workbook importer in jsdom.
jest.mock('@js-preview/excel', () => ({
  __esModule: true,
  default: require('@js-preview/excel/lib/index.umd.js'),
}));
jest.mock('@/hooks/route-hook', () => ({
  useGetKnowledgeSearchParams: () => ({}),
}));
jest.mock('@/pages/dataflow-result/hooks', () => ({
  useGetPipelineResultSearchParams: () => ({}),
}));
jest.mock('@/utils/api', () => ({ default: {}, restAPIv1: '' }));
jest.mock('@/utils/authorization-util', () => ({ getAuthorization: () => '' }));

type PreviewData = {
  rows: {
    len: number;
    getCell: (row: number, col: number) => { style: number } | null;
  };
  cols: { len: number };
  styles: { bgcolor?: string }[];
};
type PreviewInternals = {
  xs: {
    datas: PreviewData[];
    sheet: {
      verticalScrollbar: {
        move: (offset: { top: number }) => void;
        scroll: () => { top: number };
      };
    };
  };
};

it('highlights and scrolls to imported cells beyond the default grid', async () => {
  const workbook = XLSX.utils.book_new();
  const worksheet = XLSX.utils.aoa_to_sheet([['header']]);
  XLSX.utils.sheet_add_aoa(worksheet, [['wide']], { origin: 'AD2' });
  XLSX.utils.sheet_add_aoa(worksheet, [['late']], { origin: 'A150' });
  XLSX.utils.book_append_sheet(workbook, worksheet, 'Sheet1');
  const bytes = XLSX.write(workbook, { type: 'array', bookType: 'xlsx' });

  const context = new Proxy(
    { measureText: () => ({ width: 10 }) },
    {
      get: (target, key) =>
        key in target ? target[key as keyof typeof target] : () => {},
    },
  );
  jest
    .spyOn(HTMLCanvasElement.prototype, 'getContext')
    .mockReturnValue(context as unknown as CanvasRenderingContext2D);
  jest.spyOn(console, 'log').mockImplementation(() => {});
  const originalXhr = globalThis.XMLHttpRequest;
  globalThis.XMLHttpRequest = class {
    status = 200;
    response = bytes;
    onload?: () => void;
    open() {}
    setRequestHeader() {}
    send() {
      this.onload?.();
    }
  } as unknown as typeof XMLHttpRequest;

  const container = document.createElement('div');
  document.body.appendChild(container);
  const previewer = jsPreviewExcel.init(container);
  try {
    await previewer.preview('file.xlsx');
    const internal = previewer as unknown as PreviewInternals;
    const sheet = internal.xs.datas[0];
    expect(sheet.rows.len).toBeGreaterThanOrEqual(150);
    expect(sheet.cols.len).toBeGreaterThanOrEqual(30);
    const scrollbar = internal.xs.sheet.verticalScrollbar;
    const scroll = jest.spyOn(scrollbar, 'move');
    applyExcelSourceLocate(previewer, [
      [1, 150, 150, 1, 1],
      [1, 2, 2, 30, 30],
    ]);
    for (const [row, col] of [
      [149, 0],
      [1, 29],
    ]) {
      const cell = sheet.rows.getCell(row, col);
      expect(cell).not.toBeNull();
      expect(sheet.styles[cell!.style].bgcolor).toBe('#ffe58f');
    }
    expect(scroll).toHaveBeenCalledWith({ top: expect.any(Number) });
    expect(scroll.mock.calls[0][0].top).toBeGreaterThan(0);
    expect(scrollbar.scroll().top).toBe(scroll.mock.calls[0][0].top);
    // The previewer schedules its own rendering after load.
    await new Promise((resolve) => setTimeout(resolve, 30));
  } finally {
    previewer.destroy();
    container.remove();
    globalThis.XMLHttpRequest = originalXhr;
    jest.restoreAllMocks();
  }
});
