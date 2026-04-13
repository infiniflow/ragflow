import { Button } from '@/components/ui/button';
import { IDocumentDownloadInfo } from '@/interfaces/database/chat';
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
  const handleDownload = useCallback(() => {
    try {
      const byteCharacters = atob(downloadInfo.base64);
      const byteNumbers = new Array(byteCharacters.length);
      for (let i = 0; i < byteCharacters.length; i++) {
        byteNumbers[i] = byteCharacters.charCodeAt(i);
      }
      const byteArray = new Uint8Array(byteNumbers);
      const blob = new Blob([byteArray], { type: downloadInfo.mime_type });

      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = downloadInfo.filename;
      document.body.appendChild(link);
      link.click();

      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
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

const validMimeTypes = [
  'application/pdf',
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'text/plain',
  'text/markdown',
  'text/html',
];

function isDownloadInfo(value: unknown): value is DocumentDownloadInfo {
  return Boolean(
    value &&
    typeof value === 'object' &&
    'filename' in value &&
    'base64' in value &&
    'mime_type' in value &&
    validMimeTypes.includes((value as DocumentDownloadInfo).mime_type),
  );
}

function collectJsonCandidates(content: string): string[] {
  const candidates: string[] = [];
  const startPattern = /\{[^{}]*"filename"[^{}]*:/g;
  let match;

  while ((match = startPattern.exec(content)) !== null) {
    const startIndex = match.index;
    let braceCount = 0;
    let endIndex = startIndex;

    for (let i = startIndex; i < content.length; i++) {
      if (content[i] === '{') braceCount++;
      if (content[i] === '}') braceCount--;

      if (braceCount === 0) {
        endIndex = i + 1;
        break;
      }
    }

    if (endIndex > startIndex) {
      candidates.push(content.substring(startIndex, endIndex));
      startPattern.lastIndex = endIndex;
    }
  }

  return candidates;
}

export function extractDocumentDownloadInfos(
  content: string,
): DocumentDownloadInfo[] {
  const downloads: DocumentDownloadInfo[] = [];

  try {
    const parsed = JSON.parse(content);
    if (Array.isArray(parsed)) {
      parsed.forEach((item) => {
        if (isDownloadInfo(item)) {
          downloads.push(item);
        }
      });
      if (downloads.length) {
        return downloads;
      }
    } else if (isDownloadInfo(parsed)) {
      return [parsed];
    }
  } catch {
    // Fall through to mixed-content extraction.
  }

  for (const candidate of collectJsonCandidates(content)) {
    try {
      const parsed = JSON.parse(candidate);
      if (isDownloadInfo(parsed)) {
        downloads.push(parsed);
      }
    } catch {
      // Ignore invalid JSON fragments.
    }
  }

  return downloads;
}

export function extractDocumentDownloadInfo(
  content: string,
): DocumentDownloadInfo | null {
  return extractDocumentDownloadInfos(content)[0] ?? null;
}

export function mergeDocumentDownloadInfos(
  ...groups: Array<DocumentDownloadInfo[] | undefined>
): DocumentDownloadInfo[] {
  const seen = new Set<string>();

  return groups
    .flatMap((group) => group ?? [])
    .filter((info) => {
      const key = `${info.filename}::${info.mime_type}::${info.base64}`;
      if (seen.has(key)) {
        return false;
      }
      seen.add(key);
      return true;
    });
}

export function removeDocumentDownloadInfos(
  content: string,
  downloadInfos: DocumentDownloadInfo[],
): string {
  let cleaned = content;

  try {
    const parsed = JSON.parse(content);
    if (
      (Array.isArray(parsed) && parsed.every((item) => isDownloadInfo(item))) ||
      isDownloadInfo(parsed)
    ) {
      return '';
    }
  } catch {
    // Continue with string replacement.
  }

  for (const downloadInfo of downloadInfos) {
    cleaned = cleaned.replace(JSON.stringify(downloadInfo), '');
    cleaned = cleaned.replace(JSON.stringify(downloadInfo, null, 2), '');
  }

  for (const candidate of collectJsonCandidates(cleaned)) {
    try {
      const parsed = JSON.parse(candidate);
      if (isDownloadInfo(parsed)) {
        cleaned = cleaned.replace(candidate, '');
      }
    } catch {
      // Ignore invalid JSON fragments.
    }
  }

  return cleaned.trim();
}

export function removeDocumentDownloadInfo(
  content: string,
  downloadInfo: DocumentDownloadInfo,
): string {
  return removeDocumentDownloadInfos(content, [downloadInfo]);
}
