import { useGetKnowledgeSearchParams } from '@/hooks/knowledgeHook';
import { api_host } from '@/utils/api';
import { useMemo, useState } from 'react';
import { Document, Page, pdfjs } from 'react-pdf';

import 'react-pdf/dist/esm/Page/AnnotationLayer.css';
import 'react-pdf/dist/esm/Page/TextLayer.css';
import { useDocumentResizeObserver, useHighlightText } from './hooks';

import { Spin } from 'antd';
import { useGetSelectedChunk } from '../../hooks';
import styles from './index.less';

pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.js',
  import.meta.url,
).toString();

interface IProps {
  selectedChunkId: string;
}

const DocumentPreview = ({ selectedChunkId }: IProps) => {
  const [numPages, setNumPages] = useState<number>();
  const { documentId } = useGetKnowledgeSearchParams();
  const { containerWidth, setContainerRef } = useDocumentResizeObserver();
  const selectedChunk = useGetSelectedChunk(selectedChunkId);
  console.info(selectedChunk?.content_with_weight);
  const textRenderer = useHighlightText(selectedChunk?.content_with_weight);

  function onDocumentLoadSuccess({ numPages }: { numPages: number }): void {
    setNumPages(numPages);
  }

  const url = useMemo(() => {
    return `${api_host}/document/get/${documentId}`;
  }, [documentId]);

  return (
    <div ref={setContainerRef} className={styles.documentContainer}>
      <Document
        file={url}
        onLoadSuccess={onDocumentLoadSuccess}
        loading={<Spin></Spin>}
      >
        {Array.from(new Array(numPages), (el, index) => (
          <Page
            key={`page_${index + 1}`}
            pageNumber={index + 1}
            width={containerWidth}
            customTextRenderer={textRenderer}
          />
        ))}
      </Document>
    </div>
  );
};

export default DocumentPreview;
