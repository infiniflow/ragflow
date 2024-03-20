import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { IChunk, IKnowledgeFile } from '@/interfaces/database/knowledge';
import { buildChunkHighlights } from '@/utils/documentUtils';
import { useCallback, useMemo, useState } from 'react';
import { IHighlight } from 'react-pdf-highlighter';
import { useSelector } from 'umi';

export const useSelectDocumentInfo = () => {
  const documentInfo: IKnowledgeFile = useSelector(
    (state: any) => state.chunkModel.documentInfo,
  );
  return documentInfo;
};

export const useSelectChunkList = () => {
  const chunkList: IChunk[] = useSelector(
    (state: any) => state.chunkModel.data,
  );
  return chunkList;
};

export const useHandleChunkCardClick = () => {
  const [selectedChunkId, setSelectedChunkId] = useState<string>('');

  const handleChunkCardClick = useCallback((chunkId: string) => {
    setSelectedChunkId(chunkId);
  }, []);

  return { handleChunkCardClick, selectedChunkId };
};

export const useGetSelectedChunk = (selectedChunkId: string) => {
  const chunkList: IChunk[] = useSelectChunkList();
  return (
    chunkList.find((x) => x.chunk_id === selectedChunkId) ?? ({} as IChunk)
  );
};

export const useGetChunkHighlights = (
  selectedChunkId: string,
): IHighlight[] => {
  const selectedChunk: IChunk = useGetSelectedChunk(selectedChunkId);

  const highlights: IHighlight[] = useMemo(() => {
    return buildChunkHighlights(selectedChunk);
  }, [selectedChunk]);

  return highlights;
};

export const useSelectChunkListLoading = () => {
  return useOneNamespaceEffectsLoading('chunkModel', [
    'create_hunk',
    'chunk_list',
    'switch_chunk',
  ]);
};
