import { useGetKnowledgeSearchParams } from '@/hooks/knowledgeHook';
import { getDocumentFile } from '@/services/kbService';
import { api_host } from '@/utils/api';
import { useCallback, useEffect, useState } from 'react';
import { Document, Page, pdfjs } from 'react-pdf';
import { useDispatch } from 'umi';

type PDFFile = string | File | null;

pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.js',
  import.meta.url,
).toString();

const DocumentPreview = () => {
  const [numPages, setNumPages] = useState<number>();
  const [pageNumber, setPageNumber] = useState<number>(1);
  const { documentId } = useGetKnowledgeSearchParams();
  const dispatch = useDispatch();
  const [file, setFile] = useState<PDFFile>(null);

  function onDocumentLoadSuccess({ numPages }: { numPages: number }): void {
    setNumPages(numPages);
  }

  const handleChange = (e: any) => {
    console.info(e.files);
    setFile(e.target.files[0] || null);
  };

  const url = `${api_host}/document/get/${documentId}`;

  const fetch_document_file = useCallback(async () => {
    const ret: Blob = await getDocumentFile(documentId);
    console.info(ret);
    const f = new File([ret], 'xx.pdf', { type: ret.type });
    // console.info(f);
    setFile(f);
  }, [documentId]);

  useEffect(() => {
    // dispatch({ type: 'kFModel/fetch_document_file', payload: documentId });
    fetch_document_file();
  }, [fetch_document_file]);

  return (
    <div>
      {file && (
        <Document file={file} onLoadSuccess={onDocumentLoadSuccess}>
          <Page pageNumber={pageNumber} />
        </Document>
      )}

      <p>
        Page {pageNumber} of {numPages}
      </p>
      {/* <input type="file" onChange={handleChange} /> */}
    </div>
  );
};

export default DocumentPreview;
