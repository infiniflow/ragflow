import { IChunk } from '@/interfaces/database/knowledge';
import { UploadFile } from 'antd';
import { v4 as uuid } from 'uuid';

export const buildChunkHighlights = (selectedChunk: IChunk) => {
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
};

export const isFileUploadDone = (file: UploadFile) => file.status === 'done';
