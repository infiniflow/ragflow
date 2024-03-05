import { IChunk, IKnowledgeFile } from '@/interfaces/database/knowledge';
import { useCallback, useMemo, useState } from 'react';
import { IHighlight } from 'react-pdf-highlighter';
import { useSelector } from 'umi';
import { v4 as uuid } from 'uuid';

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
    return Array.isArray(selectedChunk?.positions) &&
      selectedChunk.positions.every((x) => Array.isArray(x))
      ? selectedChunk?.positions?.map((x) => {
          const actualPositions = x.map((y, index) =>
            index !== 0 ? y / 0.7 : y,
          );
          const boundingRect = {
            width: 849,
            height: 1200,
            x1: actualPositions[1],
            x2: actualPositions[2],
            y1: actualPositions[3],
            y2: actualPositions[4],
          };
          return {
            id: uuid(),
            comment: {
              text: '',
              emoji: '',
            },
            content: { text: selectedChunk.content_with_weight },
            position: {
              boundingRect: boundingRect,
              rects: [boundingRect],
              pageNumber: x[0],
            },
          };
        })
      : [];
  }, [selectedChunk]);

  return highlights;
};
