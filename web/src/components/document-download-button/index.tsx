/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { Button } from '@/components/ui/button';
import { IDocumentDownloadInfo } from '@/interfaces/database/chat';
import { downloadAgentFile } from '@/services/file-manager-service';
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
      const response = await downloadAgentFile({
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
