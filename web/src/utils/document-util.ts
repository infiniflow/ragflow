import { Images, SupportedPreviewDocumentTypes } from '@/constants/common';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import { UploadFile } from 'antd';
import { get } from 'lodash';
import { v4 as uuid } from 'uuid';

export const buildChunkHighlights = (
  selectedChunk: IChunk | IReferenceChunk,
  size: { width: number; height: number },
) => {
  return Array.isArray(selectedChunk?.positions) &&
    selectedChunk.positions.every((x) => Array.isArray(x))
    ? selectedChunk?.positions?.map((x) => {
        const boundingRect = {
          width: size.width,
          height: size.height,
          x1: x[1],
          x2: x[2],
          y1: x[3],
          y2: x[4],
        };
        return {
          id: uuid(),
          comment: {
            text: '',
            emoji: '',
          },
          content: {
            text:
              get(selectedChunk, 'content_with_weight') ||
              get(selectedChunk, 'content', ''),
          },
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

export const getExtension = (name: string) =>
  name?.slice(name.lastIndexOf('.') + 1).toLowerCase() ?? '';

export const isPdf = (name: string) => {
  return getExtension(name) === 'pdf';
};

export const getUnSupportedFilesCount = (message: string) => {
  return message.split('\n').length;
};

export const isSupportedPreviewDocumentType = (fileExtension: string) => {
  return SupportedPreviewDocumentTypes.includes(fileExtension);
};

export const isImage = (image: string) => {
  return [...Images, 'svg'].some((x) => x === image);
};
