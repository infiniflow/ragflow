import { useGetKnowledgeSearchParams } from '@/hooks/knowledgeHook';
import { api_host } from '@/utils/api';
import { useMemo, useState } from 'react';
import { Document, Page, pdfjs } from 'react-pdf';

import 'react-pdf/dist/esm/Page/AnnotationLayer.css';
import 'react-pdf/dist/esm/Page/TextLayer.css';
import { useDocumentResizeObserver } from './hooks';

import styles from './index.less';

// type PDFFile = string | File | null;

pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.js',
  import.meta.url,
).toString();

// const options = {
//   cMapUrl: '/cmaps/',
//   standardFontDataUrl: '/standard_fonts/',
// };

const DocumentPreview = () => {
  const [numPages, setNumPages] = useState<number>();
  const { documentId } = useGetKnowledgeSearchParams();
  //   const [file, setFile] = useState<PDFFile>(null);
  const { containerWidth, setContainerRef } = useDocumentResizeObserver();

  function onDocumentLoadSuccess({ numPages }: { numPages: number }): void {
    setNumPages(numPages);
  }

  //   const handleChange = (e: any) => {
  //     console.info(e.files);
  //     setFile(e.target.files[0] || null);
  //   };

  const url = useMemo(() => {
    return `${api_host}/document/get/${documentId}`;
  }, [documentId]);

  //   const fetch_document_file = useCallback(async () => {
  //     const ret: Blob = await getDocumentFile(documentId);
  //     console.info(ret);
  //     const f = new File([ret], 'xx.pdf', { type: ret.type });
  //     setFile(f);
  //   }, [documentId]);

  //   useEffect(() => {
  //     // dispatch({ type: 'kFModel/fetch_document_file', payload: documentId });
  //     fetch_document_file();
  //   }, [fetch_document_file]);

  return (
    <div ref={setContainerRef} className={styles.documentContainer}>
      <Document
        file={url}
        onLoadSuccess={onDocumentLoadSuccess}
        //   options={options}
      >
        {Array.from(new Array(numPages), (el, index) => (
          <Page
            key={`page_${index + 1}`}
            pageNumber={index + 1}
            width={containerWidth}
          />
        ))}
      </Document>
      {/* <input type="file" onChange={handleChange} /> */}
    </div>
  );
};

export default DocumentPreview;
