import jsPreviewExcel from '@js-preview/excel';
import axios from 'axios';
import mammoth from 'mammoth';
import { useCallback, useEffect, useRef, useState } from 'react';

const useFetchDocument = () => {
  const fetchDocument = useCallback((api: string) => {
    return axios.get(api, { responseType: 'arraybuffer' });
  }, []);

  return fetchDocument;
};

export const useFetchExcel = (filePath: string) => {
  const [status, setStatus] = useState(true);
  const fetchDocument = useFetchDocument();
  const containerRef = useRef<HTMLDivElement>(null);

  const fetchDocumentAsync = useCallback(async () => {
    let myExcelPreviewer;
    if (containerRef.current) {
      myExcelPreviewer = jsPreviewExcel.init(containerRef.current);
    }
    const jsonFile = await fetchDocument(filePath);
    myExcelPreviewer
      ?.preview(jsonFile.data)
      .then(() => {
        console.log('succeed');
        setStatus(true);
      })
      .catch((e) => {
        console.warn('failed', e);
        myExcelPreviewer.destroy();
        setStatus(false);
      });
  }, [filePath, fetchDocument]);

  useEffect(() => {
    fetchDocumentAsync();
  }, [fetchDocumentAsync]);

  return { status, containerRef };
};

export const useFetchDocx = (filePath: string) => {
  const [succeed, setSucceed] = useState(true);
  const fetchDocument = useFetchDocument();
  const containerRef = useRef<HTMLDivElement>(null);

  const fetchDocumentAsync = useCallback(async () => {
    const jsonFile = await fetchDocument(filePath);
    mammoth
      .convertToHtml(
        { arrayBuffer: jsonFile.data },
        { includeDefaultStyleMap: true },
      )
      .then((result) => {
        setSucceed(true);
        const docEl = document.createElement('div');
        docEl.className = 'document-container';
        docEl.innerHTML = result.value;
        const container = containerRef.current;
        if (container) {
          container.innerHTML = docEl.outerHTML;
        }
      })
      .catch((a) => {
        setSucceed(false);
        console.warn('alexei: something went wrong', a);
      });
  }, [filePath, fetchDocument]);

  useEffect(() => {
    fetchDocumentAsync();
  }, [fetchDocumentAsync]);

  return { succeed, containerRef };
};
