import message from '@/components/ui/message';
import { DocPreviewer } from '@/components/document-preview/doc-preview';
import { EpubPreviewer } from '@/components/document-preview/epub-preview';
import { ExcelCsvPreviewer } from '@/components/document-preview/excel-preview';
import { ImagePreviewer } from '@/components/document-preview/image-preview';
import Md from '@/components/document-preview/md';
import PdfPreview from '@/components/document-preview/pdf-preview';
import { PptPreviewer } from '@/components/document-preview/ppt-preview';
import { TxtPreviewer } from '@/components/document-preview/txt-preview';
import { Images } from '@/constants/common';
import { restAPIv1 } from '@/utils/api';
import { downloadFileFromBlob } from '@/utils/file-util';
import { previewHtmlFile } from '@/utils/file-util';
import request from '@/utils/request';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useSearchParams } from 'react-router';
import CSVFileViewer from '@/components/document-preview/csv-preview';

// Extract the filename from a Content-Disposition header value, or return
// undefined when the header is missing/malformed.
const parseContentDispositionFilename = (header?: string): string | undefined => {
  if (!header) return undefined;
  const match = /filename[^;=\n]*=(["']?)([^"';\n]*)\1/.exec(header);
  return match?.[2]?.trim();
};

const DocumentViewer = () => {
  const { t } = useTranslation();
  const { id: documentId } = useParams();
  const [currentQueryParameters] = useSearchParams();
  const ext = currentQueryParameters.get('ext');
  const resource =
    currentQueryParameters.get('resource') === 'files' ? 'files' : 'document';
  const api =
    resource === 'files'
      ? `${restAPIv1}/files/${documentId}`
      : `${restAPIv1}/documents/${documentId}/preview`;

  const handleDownload = useCallback(async () => {
    if (!documentId) return;
    try {
      const response = await request(api, {
        method: 'GET',
        responseType: 'blob',
      });
      const filename =
        parseContentDispositionFilename(
          response.headers['content-disposition'],
        ) ||
        (ext ? `${documentId}.${ext}` : documentId);
      downloadFileFromBlob(response.data, filename);
    } catch {
      message.error(t('message.failed'));
    }
  }, [api, documentId, ext, t]);

  const previewNode = useMemo(() => {
    if (Images.includes(ext!)) {
      return (
        <div className="flex w-full h-full items-center justify-center">
          <ImagePreviewer className="w-full !h-dvh p-5" url={api} />
        </div>
      );
    }
    switch (ext) {
      case 'md':
      case 'mdx':
        return <Md url={api} className="!h-dvh p-5" />;
      case 'txt':
      case 'eml':
        return <TxtPreviewer url={api} />;
      case 'epub':
        return <EpubPreviewer url={api} className="!h-dvh p-5" />;
      case 'pdf':
        return <PdfPreview url={api} className="!h-dvh p-5" />;
      case 'xlsx':
      case 'xls':
        return <ExcelCsvPreviewer url={api} />;
      case 'csv':
        return (
          <section className="m-1">
            <CSVFileViewer url={api} />
          </section>
        );
      case 'docx':
        return <DocPreviewer url={api} />;
      case 'ppt':
      case 'pptx':
        return <PptPreviewer url={api} className="!h-dvh p-5" />;
      default:
        return null;
    }
  }, [api, ext]);

  if (ext === 'html' && documentId) {
    previewHtmlFile(documentId, resource);
    return null;
  }

  if (!previewNode) {
    return (
      <section className="w-full h-full">
        <div className="flex h-full items-center justify-center">
          <button
            type="button"
            onClick={handleDownload}
            className="appearance-none border-0 bg-transparent p-0 text-text-primary underline"
          >
            {t('common.downloadFile')}
          </button>
        </div>
      </section>
    );
  }

  return <section className="w-full h-full">{previewNode}</section>;
};

export default DocumentViewer;
