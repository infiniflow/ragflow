import { useGetKnowledgeSearchParams } from '@/hooks/routeHook';
import { api_host } from '@/utils/api';
import { useSize } from 'ahooks';
import { CustomTextRenderer } from 'node_modules/react-pdf/dist/esm/shared/types';
import { useCallback, useEffect, useMemo, useState } from 'react';

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
  const textRenderer: CustomTextRenderer = useCallback(
    (textItem) => {
      return highlightPattern(textItem.str, searchText, textItem.pageNumber);
    },
    [searchText],
  );

  return textRenderer;
};

export const useGetDocumentUrl = () => {
  const { documentId } = useGetKnowledgeSearchParams();

  const url = useMemo(() => {
    return `${api_host}/document/get/${documentId}`;
  }, [documentId]);

  return url;
};
