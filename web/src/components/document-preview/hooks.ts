import { Authorization } from '@/constants/authorization';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { useGetPipelineResultSearchParams } from '@/pages/dataflow-result/hooks';
import api, { restAPIv1 } from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import jsPreviewExcel from '@js-preview/excel';
import { useSize } from 'ahooks';
import axios from 'axios';
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

export const useFetchExcel = (filePath: string) => {
  const [status, setStatus] = useState(true);
  const { fetchDocument } = useFetchDocument();
  const containerRef = useRef<HTMLDivElement>(null);
  const { error } = useCatchError(filePath);

  const fetchDocumentAsync = useCallback(async () => {
    let myExcelPreviewer;
    if (containerRef.current) {
      myExcelPreviewer = jsPreviewExcel.init(containerRef.current);
    }
    const jsonFile = await fetchDocument(filePath);
    myExcelPreviewer
      ?.preview(jsonFile.data)
      .then(() => {
        console.log('succeed');
        setStatus(true);
      })
      .catch((e) => {
        console.warn('failed', e);
        myExcelPreviewer.destroy();
        setStatus(false);
      });
  }, [filePath, fetchDocument]);

  useEffect(() => {
    fetchDocumentAsync();
  }, [fetchDocumentAsync]);

  return { status, containerRef, error };
};

export const useCatchDocumentError = (url: string) => {
  const httpHeaders = useMemo(() => {
    return {
      [Authorization]: getAuthorization(),
    };
  }, []);
  const [error, setError] = useState<string>('');

  const fetchDocument = useCallback(async () => {
    const { data } = await axios.get(url, { headers: httpHeaders });
    if (data.code !== 0) {
      setError(data?.message);
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
