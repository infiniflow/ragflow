import { Button } from '@/components/ui/button';
import { Download, FileText } from 'lucide-react';
import { useCallback } from 'react';

interface DocumentDownloadInfo {
  filename: string;
  base64: string;
  mime_type: string;
}

interface DocumentDownloadButtonProps {
  downloadInfo: DocumentDownloadInfo;
  className?: string;
}

export function PDFDownloadButton({
  downloadInfo,
  className,
}: DocumentDownloadButtonProps) {
  const handleDownload = useCallback(() => {
    try {
      // Convert base64 to blob
      const byteCharacters = atob(downloadInfo.base64);
      const byteNumbers = new Array(byteCharacters.length);
      for (let i = 0; i < byteCharacters.length; i++) {
        byteNumbers[i] = byteCharacters.charCodeAt(i);
      }
      const byteArray = new Uint8Array(byteNumbers);
      const blob = new Blob([byteArray], { type: downloadInfo.mime_type });

      // Create download link
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = downloadInfo.filename;
      document.body.appendChild(link);
      link.click();

      // Cleanup
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Error downloading document:', error);
    }
  }, [downloadInfo]);

  // Determine document type from mime_type or filename
  const getDocumentType = () => {
    if (downloadInfo.mime_type === 'application/pdf') return 'PDF Document';
    if (
      downloadInfo.mime_type ===
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
    )
      return 'Word Document';
    if (downloadInfo.mime_type === 'text/plain') return 'Text Document';

    // Fallback to file extension
    const ext = downloadInfo.filename.split('.').pop()?.toUpperCase();
    if (ext === 'PDF') return 'PDF Document';
    if (ext === 'DOCX') return 'Word Document';
    if (ext === 'TXT') return 'Text Document';

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

// Helper function to detect if content contains document download info
export function extractPDFDownloadInfo(
  content: string,
): DocumentDownloadInfo | null {
  try {
    // Try to parse as JSON first (for pure JSON content)
    const parsed = JSON.parse(content);
    if (parsed && parsed.filename && parsed.base64 && parsed.mime_type) {
      // Accept PDF, DOCX, and TXT formats
      const validMimeTypes = [
        'application/pdf',
        'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
        'text/plain',
      ];
      if (validMimeTypes.includes(parsed.mime_type)) {
        return parsed as DocumentDownloadInfo;
      }
    }
  } catch {
    // If direct parsing fails, try to extract JSON object from mixed content
    // Look for a JSON object that contains the required fields
    // This regex finds a balanced JSON object by counting braces
    const startPattern = /\{[^{}]*"filename"[^{}]*:/g;
    let match;

    while ((match = startPattern.exec(content)) !== null) {
      const startIndex = match.index;
      let braceCount = 0;
      let endIndex = startIndex;

      // Find the matching closing brace
      for (let i = startIndex; i < content.length; i++) {
        if (content[i] === '{') braceCount++;
        if (content[i] === '}') braceCount--;

        if (braceCount === 0) {
          endIndex = i + 1;
          break;
        }
      }

      if (endIndex > startIndex) {
        try {
          const jsonStr = content.substring(startIndex, endIndex);
          const parsed = JSON.parse(jsonStr);
          if (parsed && parsed.filename && parsed.base64 && parsed.mime_type) {
            // Accept PDF, DOCX, and TXT formats
            const validMimeTypes = [
              'application/pdf',
              'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
              'text/plain',
            ];
            if (validMimeTypes.includes(parsed.mime_type)) {
              return parsed as DocumentDownloadInfo;
            }
          }
        } catch {
          // This wasn't valid JSON, continue searching
        }
      }
    }
  }
  return null;
}

// Helper function to remove document download info from content
export function removePDFDownloadInfo(
  content: string,
  downloadInfo: DocumentDownloadInfo,
): string {
  try {
    // First, check if the entire content is just the JSON (most common case)
    try {
      const parsed = JSON.parse(content);
      if (
        parsed &&
        parsed.filename === downloadInfo.filename &&
        parsed.base64 === downloadInfo.base64
      ) {
        // The entire content is just the download JSON, return empty
        return '';
      }
    } catch {
      // Content is not pure JSON, continue with removal
    }

    // Try to remove the JSON string from content
    const jsonStr = JSON.stringify(downloadInfo);
    let cleaned = content.replace(jsonStr, '').trim();

    // Also try with pretty-printed JSON (with indentation)
    const prettyJsonStr = JSON.stringify(downloadInfo, null, 2);
    cleaned = cleaned.replace(prettyJsonStr, '').trim();

    // Also try to find and remove JSON object pattern from mixed content
    // This handles cases where the JSON might have different formatting
    const startPattern = /\{[^{}]*"filename"[^{}]*"base64"[^{}]*\}/g;
    cleaned = cleaned.replace(startPattern, '').trim();

    return cleaned;
  } catch {
    return content;
  }
}
