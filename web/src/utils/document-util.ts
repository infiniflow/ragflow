import { Images, SupportedPreviewDocumentTypes } from '@/constants/common';
import { UploadFile } from '@/interfaces/antd-compat';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/dataset';
import type { IHighlight } from 'react-pdf-highlighter';
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

/**
 * Reduce a list of `IHighlight` to the first highlight on each distinct page.
 * The PDF preview scrolls to the first highlight of each page so cross-page
 * chunks light up every page the chunk touches instead of only the first.
 */
export const firstHighlightPerPage = (highlights: IHighlight[]): IHighlight[] => {
  const seen = new Set<number>();
  return highlights.filter((h) => {
    const pn = h.position.pageNumber;
    if (seen.has(pn)) return false;
    seen.add(pn);
    return true;
  });
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
