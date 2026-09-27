/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import {
  useGetChunkHighlights,
  useGetDocumentUrl,
} from '@/hooks/use-document-request';
import { IModalProps } from '@/interfaces/common';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import { getExtension } from '@/utils/document-util';
import { get } from 'lodash';
import DocumentPreview from '../document-preview';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '../ui/sheet';

interface IProps extends IModalProps<any> {
  documentId: string;
  chunk: IChunk | IReferenceChunk;
  width?: string | number;
  height?: string | number;
}

export const PdfSheet = ({
  hideModal,
  documentId,
  chunk,
  width = '50vw',
  height,
}: IProps) => {
  const getDocumentUrl = useGetDocumentUrl(documentId);
  const url = getDocumentUrl(documentId);
  const { highlights, setWidthAndHeight } = useGetChunkHighlights(chunk);
  const fileType = getExtension(
    get(chunk, 'document_name', '') || get(chunk, 'docnm_kwd', '') || 'pdf',
  );
  const positions = Array.isArray(chunk?.positions) ? chunk.positions : [];

  return (
    <Sheet open onOpenChange={hideModal}>
      <SheetContent
        className={cn(`max-w-full`)}
        style={{
          width: width,
          height: height ? height : undefined,
        }}
      >
        <SheetHeader>
          <SheetTitle>Document Previewer</SheetTitle>
        </SheetHeader>
        {url && documentId && (
          <DocumentPreview
            className={'p-0 !h-[calc(100vh-80px)] w-full'}
            fileType={fileType || 'pdf'}
            highlights={highlights}
            setWidthAndHeight={setWidthAndHeight}
            url={url}
            positions={positions}
          />
        )}
      </SheetContent>
    </Sheet>
  );
};

export default PdfSheet;
