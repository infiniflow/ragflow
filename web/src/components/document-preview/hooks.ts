/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { Authorization } from '@/constants/authorization';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { useGetPipelineResultSearchParams } from '@/pages/dataflow-result/hooks';
import api, { restAPIv1 } from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import jsPreviewExcel from '@js-preview/excel';
import { useDebounceFn, useSize } from 'ahooks';
import axios from 'axios';
import JSZip from 'jszip';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import * as XLSX from 'xlsx';

// ZIP file header bytes "PK"
const ZIP_HEADER_0 = 0x50;
const ZIP_HEADER_1 = 0x4b;

export const isZipLikeBlob = async (blob: Blob): Promise<boolean> => {
  try {
    const headerSlice = blob.slice(0, 4);
    const buf = await headerSlice.arrayBuffer();
    const bytes = new Uint8Array(buf);
    return (
      bytes.length >= 2 &&
      bytes[0] === ZIP_HEADER_0 &&
      bytes[1] === ZIP_HEADER_1
    );
  } catch (e) {
    console.error('Failed to inspect blob header', e);
    return false;
  }
};

export const useDocumentResizeObserver = () => {
  const [containerWidth, setContainerWidth] = useState<number>();
  const [containerRef, setContainerRef] = useState<HTMLElement | null>(null);
  const size = useSize(containerRef);

  const onResize = useCallback((width?: number) => {
    if (width) {
      setContainerWidth(width);
    }
  }, []);

  useEffect(() => {
    onResize(size?.width);
  }, [size?.width, onResize]);

  return { containerWidth, setContainerRef };
};

function highlightPattern(text: string, pattern: string, pageNumber: number) {
  if (pageNumber === 2) {
    return `<mark>${text}</mark>`;
  }
  if (text.trim() !== '' && pattern.match(text)) {
    // return pattern.replace(text, (value) => `<mark>${value}</mark>`);
    return `<mark>${text}</mark>`;
  }
  return text.replace(pattern, (value) => `<mark>${value}</mark>`);
}

export const useHighlightText = (searchText: string = '') => {
  const textRenderer = useCallback(
    (textItem: any) => {
      return highlightPattern(textItem.str, searchText, textItem.pageNumber);
    },
    [searchText],
  );

  return textRenderer;
};

export const useGetDocumentUrl = (isAgent: boolean) => {
  const { documentId } = useGetKnowledgeSearchParams();
  const { createdBy, documentId: id } = useGetPipelineResultSearchParams();

  const url = useMemo(() => {
    if (isAgent) {
      return api.downloadFile + `?id=${id}&created_by=${createdBy}`;
    }
    return `${restAPIv1}/documents/${documentId}/preview`;
  }, [createdBy, documentId, id, isAgent]);

  return url;
};

export const useFetchDocument = () => {
  const fetchDocument = useCallback(async (api: string) => {
    const ret = await axios.get(api, {
      headers: {
        [Authorization]: getAuthorization(),
      },
      responseType: 'arraybuffer',
    });
    return ret;
  }, []);

  return { fetchDocument };
};

/**
 * WPS spreadsheets embed images in cells via the proprietary DISPIMG formula
 * (stored in xl/cellimages.xml). @js-preview/excel cannot evaluate this
 * formula and crashes with "Cannot read properties of undefined (reading
 * 'render')". This function strips DISPIMG formulas from worksheet XML,
 * replacing them with a text placeholder so the rest of the sheet renders.
 */
async function stripWpsDispImg(data: ArrayBuffer): Promise<ArrayBuffer> {
  const zip = await JSZip.loadAsync(data);

  const worksheetPaths = Object.keys(zip.files).filter((path) =>
    /^xl\/worksheets\/sheet\d+\.xml$/.test(path),
  );

  let modified = false;
  for (const path of worksheetPaths) {
    const file = zip.file(path);
    if (!file) continue;
    const xml = await file.async('string');
    if (!xml.includes('DISPIMG')) continue;

    modified = true;
    let cleaned = xml;
    // Remove <f> formula tags that reference DISPIMG
    cleaned = cleaned.replace(/<f\b[^>]*>[\s\S]*?DISPIMG[\s\S]*?<\/f>/g, '');
    // Replace cached <v> values that reference DISPIMG with a placeholder
    cleaned = cleaned.replace(/<v>[^<]*DISPIMG[^<]*<\/v>/g, '<v>[图片]</v>');
    zip.file(path, cleaned);
  }

  if (!modified) return data;
  return zip.generateAsync({
    type: 'arraybuffer',
    compression: 'DEFLATE',
  });
}

/**
 * ExcelJS (used internally by @js-preview/excel) fails to parse workbooks
 * whose root <workbook> element carries an XML namespace prefix (e.g.
 * <x:workbook> instead of <workbook>). This is common in files exported by
 * WPS Office or older Excel versions. The workbook-xform parser only sets
 * its model when the closing tag name is exactly "workbook", so a prefixed
 * root element leaves the model undefined and crashes with
 * "Cannot read properties of undefined (reading 'sheets')".
 *
 * When such a prefix is detected, re-serialize the file with SheetJS (which
 * always emits a standard, prefix-free xlsx) so ExcelJS can parse it.
 */
async function normalizeXlsxForExcelJS(
  data: ArrayBuffer,
): Promise<ArrayBuffer> {
  try {
    const zip = await JSZip.loadAsync(data);
    const workbookFile = zip.file('xl/workbook.xml');
    if (!workbookFile) return data;

    const xml = await workbookFile.async('string');
    // Detect a namespace prefix on the root <workbook> element, e.g. <x:workbook>
    if (!/<\w+:workbook[\s>]/.test(xml)) return data;

    const workbook = XLSX.read(data, { type: 'array' });
    return XLSX.write(workbook, {
      bookType: 'xlsx',
      type: 'array',
    }) as ArrayBuffer;
  } catch {
    // Not a valid ZIP, SheetJS can't parse, etc. - let the previewer
    // handle the original data and surface its own error.
    return data;
  }
}

/**
 * x-spreadsheet (used internally by @js-preview/excel) throws
 * "Worksheet name ... cannot include any of the following characters:
 * * ? : \ / [ ]" when a sheet name contains any of those characters. Files
 * generated programmatically (not through Excel/WPS UI) can carry such names
 * (e.g. "Visible:Data"), crashing the whole preview. Replace the illegal
 * characters with underscores in xl/workbook.xml so the previewer renders.
 */
async function sanitizeSheetNames(data: ArrayBuffer): Promise<ArrayBuffer> {
  const zip = await JSZip.loadAsync(data);
  const workbookFile = zip.file('xl/workbook.xml');
  if (!workbookFile) return data;

  const xml = await workbookFile.async('string');
  const cleaned = xml.replace(
    /(<sheet\b[^>]*\bname=")([^"]*)(")/g,
    (_match, prefix: string, name: string, suffix: string) =>
      prefix + name.replace(/[:\\/?*[\]]/g, '_') + suffix,
  );
  if (cleaned === xml) return data;

  zip.file('xl/workbook.xml', cleaned);
  return zip.generateAsync({
    type: 'arraybuffer',
    compression: 'DEFLATE',
  });
}

type ExcelLocatePos = number[];

/** x-spreadsheet Data instance used by @js-preview/excel (not a plain object). */
type XsData = {
  addStyle: (style: Record<string, unknown>) => number;
  rows: {
    setCell: (
      ri: number,
      ci: number,
      cell: { style?: number; text?: string },
      type?: 'all' | 'text' | 'format',
    ) => void;
    getCellOrNew: (ri: number, ci: number) => { style?: number; text?: string };
    getHeight?: (ri: number) => number;
    len?: number;
  };
  cols?: { len?: number };
  scroll?: { x: number; y: number };
};

type XsInstance = {
  datas: XsData[];
  reRender?: () => void;
  sheet: {
    resetData: (data: XsData) => void;
    data?: XsData & { row?: { height?: number } };
    reload?: () => void;
    table?: { render?: () => void };
  };
  bottombar?: {
    items: HTMLElement[];
    clickSwap2?: (el: HTMLElement) => void;
  };
};

/** Locate inside @js-preview/excel: switch sheet, paint range via Data API, scroll. */
export function applyExcelSourceLocate(
  previewer: ReturnType<typeof jsPreviewExcel.init>,
  pos: ExcelLocatePos,
) {
  if (!pos || pos.length < 5) return;
  const xs = (previewer as { xs?: XsInstance }).xs;
  if (!xs?.datas?.length) return;

  const sheetIdx = Math.min(
    xs.datas.length - 1,
    Math.max(0, (pos[0] || 1) - 1),
  );
  let r1 = Math.max(0, (pos[1] || 1) - 1);
  let r2 = Math.max(r1, (pos[2] || pos[1] || 1) - 1);
  let c1 = (pos[3] || 1) - 1;
  let c2 = (pos[4] || pos[3] || 1) - 1;
  if (c1 < 0) c1 = 0;
  if (c2 < c1) c2 = c1;

  const data = xs.datas[sheetIdx];
  if (!data?.addStyle || !data.rows?.setCell) return;

  if (typeof data.rows.len === 'number') {
    const lastRow = Math.max(0, data.rows.len - 1);
    r1 = Math.min(r1, lastRow);
    r2 = Math.min(r2, lastRow);
  }
  if (typeof data.cols?.len === 'number') {
    const lastCol = Math.max(0, data.cols.len - 1);
    c1 = Math.min(c1, lastCol);
    c2 = Math.min(c2, lastCol);
  }

  const theme = getComputedStyle(document.documentElement);
  if (r2 >= r1 && c2 >= c1) {
    const styleIdx = data.addStyle({
      bgcolor:
        theme.getPropertyValue('--background-highlight').trim() || '#ffe58f',
      color: theme.getPropertyValue('--text-title').trim() || '#1a1a1a',
    });
    for (let r = r1; r <= r2; r++) {
      for (let c = c1; c <= c2; c++) {
        data.rows.setCell(r, c, { style: styleIdx }, 'format');
      }
    }
  }

  // clickSwap2 must run even when the target sheet is already on screen: it is
  // the library's only hook that redraws images embedded in the sheet, and the
  // reRender/render/reload calls below each clear the canvas first.
  const tab = xs.bottombar?.items?.[sheetIdx];
  if (tab && typeof xs.bottombar?.clickSwap2 === 'function') {
    xs.bottombar.clickSwap2(tab);
  } else {
    xs.sheet.resetData(data);
  }

  const rowHeight =
    data.rows.getHeight?.(0) || xs.sheet.data?.row?.height || 24;
  let y = 0;
  for (let r = 0; r < r1; r++) {
    y += data.rows.getHeight?.(r) ?? rowHeight;
  }
  data.scroll = { ...(data.scroll || { x: 0, y: 0 }), x: 0, y };
  if (xs.sheet.data && xs.sheet.data !== data) {
    xs.sheet.data.scroll = {
      ...(xs.sheet.data.scroll || { x: 0, y: 0 }),
      x: 0,
      y,
    };
  }
  xs.reRender?.();
  xs.sheet.table?.render?.();
  xs.sheet.reload?.();
}

export const useFetchExcel = (filePath: string, positions?: number[][]) => {
  const [status, setStatus] = useState(true);
  const { fetchDocument } = useFetchDocument();
  const [containerEl, setContainerEl] = useState<HTMLDivElement | null>(null);
  const size = useSize(containerEl);

  const previewerRef = useRef<ReturnType<typeof jsPreviewExcel.init> | null>(
    null,
  );
  // Cache the fetched ArrayBuffer so we don't re-fetch on every resize.
  const dataRef = useRef<ArrayBuffer | null>(null);
  const [dataReady, setDataReady] = useState(false);
  const positionsRef = useRef(positions);
  positionsRef.current = positions;
  const locateKey = positions?.[0]?.join(',') ?? '';
  const locateTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // @js-preview/excel reads the container's width/height at init time and
  // exposes no public resize method, so the only way to make the spreadsheet
  // track container size changes is to destroy + re-init the previewer.
  const renderPreview = useCallback(async () => {
    if (!containerEl || !dataRef.current) return;

    if (locateTimerRef.current != null) {
      clearTimeout(locateTimerRef.current);
      locateTimerRef.current = null;
    }
    if (previewerRef.current) {
      previewerRef.current.destroy();
      previewerRef.current = null;
    }

    const previewer = jsPreviewExcel.init(containerEl);
    previewerRef.current = previewer;

    try {
      await previewer.preview(dataRef.current);
      const pos = positionsRef.current?.[0];
      if (pos) {
        // bottombar tabs are created during loadData; apply after paint.
        const run = () => {
          if (previewerRef.current !== previewer) return;
          applyExcelSourceLocate(previewer, pos);
        };
        requestAnimationFrame(() => {
          run();
          locateTimerRef.current = setTimeout(run, 50);
        });
      }
      setStatus(true);
    } catch (e) {
      // oxlint-disable-next-line no-console
      console.warn('failed', e);
      // Defer destroy() so the library's pending setTimeout(0) callback
      // (scheduled by xs.loadData during its error handling) can access
      // workbookDataSource before we null it out. Destroying immediately
      // triggers a secondary "Cannot read properties of null (reading
      // '_worksheets')" error.
      previewerRef.current = null;
      setTimeout(() => {
        previewer.destroy();
      }, 0);
      setStatus(false);
    }
  }, [containerEl]);

  // Leading edge renders immediately on first data/size, trailing edge
  // collapses rapid resize bursts into a single re-render.
  const { run: debouncedRender } = useDebounceFn(renderPreview, {
    wait: 150,
    leading: true,
    trailing: true,
  });

  // Fetch the file once per url. On url change, drop cached data and the
  // existing previewer so the next render starts from a clean state.
  useEffect(() => {
    if (!filePath) return;

    setDataReady(false);
    setStatus(true);
    if (previewerRef.current) {
      previewerRef.current.destroy();
      previewerRef.current = null;
    }
    dataRef.current = null;

    let cancelled = false;
    (async () => {
      try {
        const jsonFile = await fetchDocument(filePath);
        if (cancelled) return;
        try {
          let data = jsonFile.data;
          data = await normalizeXlsxForExcelJS(data);
          data = await stripWpsDispImg(data);
          data = await sanitizeSheetNames(data);
          dataRef.current = data;
        } catch {
          // Not a valid ZIP or preprocessing failed — use original data
          dataRef.current = jsonFile.data;
        }
        setDataReady(true);
      } catch (e) {
        if (!cancelled) {
          // oxlint-disable-next-line no-console
          console.warn('failed to fetch', e);
          setStatus(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [filePath, fetchDocument]);

  // Initial render + debounced re-render on container resize / locate target change.
  useEffect(() => {
    if (!dataReady || !containerEl) return;
    debouncedRender();
  }, [
    dataReady,
    containerEl,
    size?.width,
    size?.height,
    locateKey,
    debouncedRender,
  ]);

  // Tear down the previewer on unmount.
  useEffect(() => {
    return () => {
      if (locateTimerRef.current != null) {
        clearTimeout(locateTimerRef.current);
        locateTimerRef.current = null;
      }
      if (previewerRef.current) {
        previewerRef.current.destroy();
        previewerRef.current = null;
      }
    };
  }, []);

  return { status, containerRef: setContainerEl };
};

export const useCatchDocumentError = (url: string) => {
  const httpHeaders = useMemo(() => {
    return {
      [Authorization]: getAuthorization(),
    };
  }, []);
  const [error, setError] = useState<string>('');

  const fetchDocument = useCallback(async () => {
    try {
      const { data } = await axios.get(url, { headers: httpHeaders });
      // Only treat as error if response is JSON with an error code
      // Binary data (like PDF) won't have a code property
      if (
        data &&
        typeof data === 'object' &&
        'code' in data &&
        data.code !== 0
      ) {
        setError(data?.message || 'Failed to load document');
      }
    } catch (e) {
      // Network errors or non-2xx responses
      const errMsg = e instanceof Error ? e.message : 'Failed to load document';
      if (errMsg) {
        setError(errMsg);
      }
    }
  }, [url, httpHeaders]);
  useEffect(() => {
    fetchDocument();
  }, [fetchDocument]);

  return error;
};

const ZOOM_STEPS = [25, 50, 75, 100, 125, 150, 175, 200] as const;

const clampZoom = (scale: number, direction: 1 | -1): number => {
  const exactIdx = ZOOM_STEPS.indexOf(scale as (typeof ZOOM_STEPS)[number]);
  let idx: number;

  if (exactIdx >= 0) {
    // Already on a predefined step: move one step in the zoom direction.
    idx = exactIdx + direction;
  } else if (direction > 0) {
    // Between steps and zooming in: snap up to the next higher step. This
    // index is already the target, so it must not be advanced again.
    const next = ZOOM_STEPS.findIndex((v) => v > scale);
    idx = next < 0 ? ZOOM_STEPS.length - 1 : next;
  } else {
    // Between steps and zooming out: snap down to the next lower step.
    let prev = 0;
    for (let i = ZOOM_STEPS.length - 1; i >= 0; i--) {
      if (ZOOM_STEPS[i] < scale) {
        prev = i;
        break;
      }
    }
    idx = prev;
  }

  idx = Math.max(0, Math.min(ZOOM_STEPS.length - 1, idx));
  return ZOOM_STEPS[idx] ?? scale;
};

interface UseDocxPreviewZoomOptions {
  url: string;
  totalPages: number;
  pageWidthPx?: number;
  containerWidth?: number;
  paddingPx?: number;
  enabled?: boolean;
}

interface UseDocxPreviewZoomResult {
  zoomScale: number;
  minZoom: number;
  maxZoom: number;
  handleZoomIn: () => void;
  handleZoomOut: () => void;
  resetZoom: () => void;
}

export const useDocxPreviewZoom = ({
  url,
  totalPages,
  pageWidthPx,
  containerWidth,
  paddingPx = 32,
  enabled = true,
}: UseDocxPreviewZoomOptions): UseDocxPreviewZoomResult => {
  const [zoomScale, setZoomScale] = useState(100);
  const [hasUserZoomed, setHasUserZoomed] = useState(false);
  const [isInitialFitPending, setIsInitialFitPending] = useState(true);

  const resetZoom = useCallback(() => {
    setZoomScale(100);
    setHasUserZoomed(false);
    setIsInitialFitPending(true);
  }, []);

  useEffect(() => {
    resetZoom();
  }, [url, resetZoom]);

  const handleZoomIn = useCallback(() => {
    setHasUserZoomed(true);
    setZoomScale((s) => clampZoom(s, 1));
  }, []);

  const handleZoomOut = useCallback(() => {
    setHasUserZoomed(true);
    setZoomScale((s) => clampZoom(s, -1));
  }, []);

  // Fit the page width to the container on first paint and on resize,
  // unless the user has manually changed the zoom level.
  useEffect(() => {
    if (!enabled || totalPages <= 0 || !containerWidth || !pageWidthPx) {
      return;
    }

    const availableWidth = Math.max(0, containerWidth - paddingPx);
    if (availableWidth <= 0) {
      return;
    }

    const fitScale = Math.floor((availableWidth / pageWidthPx) * 100);
    const clampedFitScale = Math.min(100, fitScale);

    if (isInitialFitPending) {
      setZoomScale(clampedFitScale);
      setIsInitialFitPending(false);
    } else if (!hasUserZoomed) {
      setZoomScale(clampedFitScale);
    }
  }, [
    enabled,
    totalPages,
    containerWidth,
    pageWidthPx,
    paddingPx,
    isInitialFitPending,
    hasUserZoomed,
  ]);

  return {
    zoomScale,
    minZoom: ZOOM_STEPS[0],
    maxZoom: ZOOM_STEPS[ZOOM_STEPS.length - 1],
    handleZoomIn,
    handleZoomOut,
    resetZoom,
  };
};
