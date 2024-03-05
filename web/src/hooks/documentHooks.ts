import { IChunk } from '@/interfaces/database/knowledge';
import { api_host } from '@/utils/api';
import { buildChunkHighlights } from '@/utils/documentUtils';
import { useMemo } from 'react';
import { IHighlight } from 'react-pdf-highlighter';

export const useGetDocumentUrl = (documentId: string) => {
  const url = useMemo(() => {
    return `${api_host}/document/get/${documentId}`;
  }, [documentId]);

  return url;
};

export const useGetChunkHighlights = (selectedChunk: IChunk): IHighlight[] => {
  const highlights: IHighlight[] = useMemo(() => {
    return buildChunkHighlights(selectedChunk);
  }, [selectedChunk]);

  return highlights;
};
