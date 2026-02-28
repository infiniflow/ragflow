import { Images } from '@/constants/common';
import { api_host } from '@/utils/api';
import { useParams, useSearchParams } from 'react-router';
// import Docx from './docx';
// import Excel from './excel';
// import Image from './image';
// import Md from './md';
// import Pdf from './pdf';
// import Text from './text';

import { DocPreviewer } from '@/components/document-preview/doc-preview';
import { ExcelCsvPreviewer } from '@/components/document-preview/excel-preview';
import { ImagePreviewer } from '@/components/document-preview/image-preview';
import Md from '@/components/document-preview/md';
import PdfPreview from '@/components/document-preview/pdf-preview';
import { PptPreviewer } from '@/components/document-preview/ppt-preview';
import { TxtPreviewer } from '@/components/document-preview/txt-preview';
import { previewHtmlFile } from '@/utils/file-util';
// import styles from './index.less';

// TODO: The interface returns an incorrect content-type for the SVG.

const DocumentViewer = () => {
  const { id: documentId } = useParams();
  const [currentQueryParameters] = useSearchParams();
  const ext = currentQueryParameters.get('ext');
  const prefix = currentQueryParameters.get('prefix');
  const api = `${api_host}/${prefix || 'file'}/get/${documentId}`;
  // request.head

  if (ext === 'html' && documentId) {
    previewHtmlFile(documentId);
    return;
  }

  return (
    <section className="w-full h-full">
      {Images.includes(ext!) && (
        <div className="flex w-full h-full items-center justify-center">
          {/* <Image src={api} preview={false}></Image> */}
          <ImagePreviewer className="w-full !h-dvh p-5" url={api} />
        </div>
      )}
      {(ext === 'md' || ext === 'mdx') && (
        <Md url={api} className="!h-dvh p-5"></Md>
      )}
      {ext === 'txt' && <TxtPreviewer url={api}></TxtPreviewer>}

      {ext === 'pdf' && (
        <PdfPreview url={api} className="!h-dvh p-5"></PdfPreview>
      )}
      {(ext === 'xlsx' || ext === 'xls') && (
        <ExcelCsvPreviewer url={api}></ExcelCsvPreviewer>
      )}

      {ext === 'docx' && <DocPreviewer url={api}></DocPreviewer>}

      {(ext === 'ppt' || ext === 'pptx') && (
        <PptPreviewer url={api} className="!h-dvh p-5"></PptPreviewer>
      )}
    </section>
  );
};

export default DocumentViewer;
