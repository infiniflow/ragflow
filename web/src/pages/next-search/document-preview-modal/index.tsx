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

import DocumentPreview from '@/components/document-preview';
import { FileIcon } from '@/components/icon-font';
import { Modal } from '@/components/ui/modal/modal';
import {
  useGetChunkHighlights,
  useGetDocumentUrl,
} from '@/hooks/use-document-request';
import { IModalProps } from '@/interfaces/common';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';

interface IProps extends IModalProps<any> {
  documentId: string;
  chunk: {
    docnm_kwd?: string;
    document_name?: string;
    positions?: number[][];
    content_with_weight?: string;
    content?: string | null;
    [key: string]: any;
  };
}
function getFileExtensionRegex(filename: string): string {
  const match = filename.match(/\.([^.]+)$/);
  return match ? match[1].toLowerCase() : '';
}
const PdfDrawer = ({
  visible = false,
  hideModal,
  documentId,
  chunk,
}: IProps) => {
  const getDocumentUrl = useGetDocumentUrl(documentId);
  const { highlights, setWidthAndHeight } = useGetChunkHighlights(
    chunk as IChunk | IReferenceChunk,
  );
  // const ref = useRef<(highlight: IHighlight) => void>(() => {});
  // const [loaded, setLoaded] = useState(false);
  const documentName = chunk.docnm_kwd || chunk.document_name;
  const fileType = documentName ? getFileExtensionRegex(documentName) : '';
  const isWebPage = !fileType && !!chunk.document_url;
  const url = isWebPage ? (chunk.document_url as string) : getDocumentUrl();
  const positions = Array.isArray(chunk.positions) ? chunk.positions : [];
  return (
    <Modal
      title={
        <div className="flex items-center gap-2">
          <FileIcon name={documentName as string}></FileIcon>
          {isWebPage ? (
            <a
              href={url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-text-sub-title-invert underline"
            >
              {documentName}
            </a>
          ) : (
            documentName
          )}
        </div>
      }
      onCancel={hideModal}
      open={visible}
      showfooter={false}
    >
      <DocumentPreview
        className={cn(
          '!h-[calc(100dvh-300px)] overflow-auto border-none padding-0 max-h-full',
        )}
        fileType={fileType}
        highlights={highlights}
        setWidthAndHeight={setWidthAndHeight}
        url={url}
        positions={positions}
      ></DocumentPreview>
    </Modal>
  );
};

export default PdfDrawer;
