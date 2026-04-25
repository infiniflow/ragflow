import { Button } from '@/components/ui/button';
import { IDocumentDownloadInfo } from '@/interfaces/database/chat';
import { downloadFile } from '@/services/file-manager-service';
import { downloadFileFromBlob } from '@/utils/file-util';
import { Download, FileText } from 'lucide-react';
import { useCallback } from 'react';

export type DocumentDownloadInfo = IDocumentDownloadInfo;

interface DocumentDownloadButtonProps {
  downloadInfo: DocumentDownloadInfo;
  className?: string;
}

export function DocumentDownloadButton({
  downloadInfo,
  className,
}: DocumentDownloadButtonProps) {
  const handleDownload = useCallback(async () => {
    try {
      const ext =
        downloadInfo.filename.split('.').pop()?.toLowerCase() || 'bin';
      const response = await downloadFile({
        docId: downloadInfo.doc_id,
        ext,
      });
      const blob = new Blob([response.data], {
        type: downloadInfo.mime_type || response.data.type,
      });
      downloadFileFromBlob(blob, downloadInfo.filename);
    } catch (error) {
      console.error('Error downloading document:', error);
    }
  }, [downloadInfo]);

  const getDocumentType = () => {
    if (downloadInfo.mime_type === 'application/pdf') return 'PDF Document';
    if (
      downloadInfo.mime_type ===
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
    )
      return 'Word Document';
    if (
      downloadInfo.mime_type ===
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
    )
      return 'Excel Document';
    if (downloadInfo.mime_type === 'text/plain') return 'Text Document';
    if (downloadInfo.mime_type === 'text/markdown') return 'Markdown Document';
    if (downloadInfo.mime_type === 'text/html') return 'HTML Document';

    const ext = downloadInfo.filename.split('.').pop()?.toUpperCase();
    if (ext === 'PDF') return 'PDF Document';
    if (ext === 'DOCX') return 'Word Document';
    if (ext === 'XLSX') return 'Excel Document';
    if (ext === 'TXT') return 'Text Document';
    if (ext === 'MD') return 'Markdown Document';
    if (ext === 'HTML' || ext === 'HTM') return 'HTML Document';

    return 'Document';
  };

  return (
    <div
      className={`flex items-center gap-3 p-4 border rounded-lg bg-background-card ${className || ''}`}
    >
      <div className="flex-shrink-0">
        <div className="p-2 bg-accent-primary/10 rounded-lg">
          <FileText className="w-6 h-6 text-accent-primary" />
        </div>
      </div>
      <div className="flex-1 min-w-0">
        <div className="font-medium text-sm truncate">
          {downloadInfo.filename}
        </div>
        <div className="text-xs text-muted-foreground">{getDocumentType()}</div>
      </div>
      <Button
        onClick={handleDownload}
        size="sm"
        className="flex items-center gap-2"
      >
        <Download className="w-4 h-4" />
        Download
      </Button>
    </div>
  );
}
