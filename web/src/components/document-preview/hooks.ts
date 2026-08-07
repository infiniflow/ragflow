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

export const useCatchError = (api: string) => {
  const [error, setError] = useState('');
  const fetchDocument = useCallback(async () => {
    const ret = await axios.get(api);
    const { data } = ret;
    if (!(data instanceof ArrayBuffer) && data.code !== 0) {
      setError(data.message);
    }
    return ret;
  }, [api]);

  useEffect(() => {
    fetchDocument();
  }, [fetchDocument]);

  return { fetchDocument, error };
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
    cleaned = cleaned.replace(
      /<f\b[^>]*>[\s\S]*?DISPIMG[\s\S]*?<\/f>/g,
      '',
    );
    // Replace cached <v> values that reference DISPIMG with a placeholder
    cleaned = cleaned.replace(
      /<v>[^<]*DISPIMG[^<]*<\/v>/g,
      '<v>[图片]</v>',
    );
    zip.file(path, cleaned);
  }

  if (!modified) return data;
  return zip.generateAsync({
    type: 'arraybuffer',
    compression: 'DEFLATE',
  });
}

export const useFetchExcel = (filePath: string) => {
  const [status, setStatus] = useState(true);
  const { fetchDocument } = useFetchDocument();
  const [containerEl, setContainerEl] = useState<HTMLDivElement | null>(null);
  const size = useSize(containerEl);
  const { error } = useCatchError(filePath);

  const previewerRef = useRef<ReturnType<typeof jsPreviewExcel.init> | null>(
    null,
  );
  // Cache the fetched ArrayBuffer so we don't re-fetch on every resize.
  const dataRef = useRef<ArrayBuffer | null>(null);
  const [dataReady, setDataReady] = useState(false);

  // @js-preview/excel reads the container's width/height at init time and
  // exposes no public resize method, so the only way to make the spreadsheet
  // track container size changes is to destroy + re-init the previewer.
  const renderPreview = useCallback(async () => {
    if (!containerEl || !dataRef.current) return;

    if (previewerRef.current) {
      previewerRef.current.destroy();
      previewerRef.current = null;
    }

    const previewer = jsPreviewExcel.init(containerEl);
    previewerRef.current = previewer;

    try {
      await previewer.preview(dataRef.current);
      setStatus(true);
    } catch (e) {
      // oxlint-disable-next-line no-console
      console.warn('failed', e);
      previewer.destroy();
      previewerRef.current = null;
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
          dataRef.current = await stripWpsDispImg(jsonFile.data);
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

  // Initial render + debounced re-render on container resize.
  useEffect(() => {
    if (!dataReady || !containerEl) return;
    debouncedRender();
  }, [dataReady, containerEl, size?.width, size?.height, debouncedRender]);

  // Tear down the previewer on unmount.
  useEffect(() => {
    return () => {
      if (previewerRef.current) {
        previewerRef.current.destroy();
        previewerRef.current = null;
      }
    };
  }, []);

  return { status, containerRef: setContainerEl, error };
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
  let idx = ZOOM_STEPS.indexOf(scale as (typeof ZOOM_STEPS)[number]);
  if (idx < 0) {
    if (direction > 0) {
      idx = ZOOM_STEPS.findIndex((v) => v > scale);
    } else {
      for (let i = ZOOM_STEPS.length - 1; i >= 0; i--) {
        if (ZOOM_STEPS[i] < scale) {
          idx = i;
          break;
        }
      }
    }
  }
  idx = Math.max(
    0,
    Math.min(ZOOM_STEPS.length - 1, idx < 0 ? 0 : idx + direction),
  );
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
