import { FileIcon } from '@/components/icon-font';
import { Modal } from '@/components/ui/modal/modal';
import {
  useGetChunkHighlights,
  useGetDocumentUrl,
} from '@/hooks/document-hooks';
import { IModalProps } from '@/interfaces/common';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import DocumentPreview from '@/pages/chunk/parsed-result/add-knowledge/components/knowledge-chunk/components/document-preview';
import { useEffect, useState } from 'react';

interface IProps extends IModalProps<any> {
  documentId: string;
  chunk: IChunk | IReferenceChunk;
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
  const { highlights, setWidthAndHeight } = useGetChunkHighlights(chunk);
  // const ref = useRef<(highlight: IHighlight) => void>(() => {});
  // const [loaded, setLoaded] = useState(false);
  const url = getDocumentUrl();

  console.log('chunk--->', chunk.docnm_kwd, url);
  const [fileType, setFileType] = useState('');

  useEffect(() => {
    if (chunk.docnm_kwd) {
      const type = getFileExtensionRegex(chunk.docnm_kwd);
      setFileType(type);
    }
  }, [chunk.docnm_kwd]);
  return (
    <Modal
      title={
        <div className="flex items-center gap-2">
          <FileIcon name={chunk.docnm_kwd}></FileIcon>
          {chunk.docnm_kwd}
        </div>
      }
      onCancel={hideModal}
      open={visible}
      showfooter={false}
    >
      <DocumentPreview
        className={'!h-[calc(100dvh-300px)] overflow-auto'}
        fileType={fileType}
        highlights={highlights}
        setWidthAndHeight={setWidthAndHeight}
        url={url}
      ></DocumentPreview>
    </Modal>
  );
};

export default PdfDrawer;
