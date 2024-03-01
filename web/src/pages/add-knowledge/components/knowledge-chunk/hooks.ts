import { IChunk, IKnowledgeFile } from '@/interfaces/database/knowledge';
import { useCallback, useState } from 'react';
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
  return chunkList.find((x) => x.chunk_id === selectedChunkId);
};
