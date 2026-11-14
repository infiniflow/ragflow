'use client';

import {
  FileUpload,
  FileUploadDropzone,
  FileUploadItem,
  FileUploadItemDelete,
  FileUploadItemMetadata,
  FileUploadItemPreview,
  FileUploadItemProgress,
  FileUploadList,
  FileUploadTrigger,
  type FileUploadProps,
} from '@/components/file-upload';
import { Button } from '@/components/ui/button';
import { useUploadAgentFile } from '@/hooks/use-agent-request';
import { Upload, X } from 'lucide-react';
import * as React from 'react';
import { toast } from 'sonner';

type FileUploadDirectUploadProps = {
  value: Record<string, any> | Record<string, any>[];
  onChange(value: Record<string, any>[]): void;
  maxFiles?: number;
};

/**
 * Uses value as initial upload metadata and emits it with successful current selections.
 * Each selection owns a record so removed or replaced uploads cannot change the field.
 */
export function FileUploadDirectUpload({
  value,
  onChange,
  maxFiles,
}: FileUploadDirectUploadProps) {
  const [files, setFiles] = React.useState<File[]>([]);
  const initialUploadedFilesRef = React.useRef<Record<string, any>[]>(
    Array.isArray(value) ? value : value ? [value] : [],
  );
  const uploadedFilesRef = React.useRef(
    new Map<File, { data?: Record<string, any> }>(),
  );

  const { uploadAgentFile } = useUploadAgentFile();

  /** Reports initial metadata plus successful current selections in selection order. */
  const notifyUploadedFilesChange = React.useCallback(() => {
    onChange([
      ...initialUploadedFilesRef.current,
      ...Array.from(uploadedFilesRef.current.values()).flatMap(({ data }) =>
        data ? [data] : [],
      ),
    ]);
  }, [onChange]);

  /** Uploads concurrently, ignoring callbacks whose selection record is no longer current. */
  const onUpload: NonNullable<FileUploadProps['onUpload']> = React.useCallback(
    async (files, { onSuccess, onError }) => {
      try {
        const uploadPromises = files.map(async (file) => {
          const upload = uploadedFilesRef.current.get(file);
          if (!upload) return;

          const handleError = (error?: any) => {
            if (uploadedFilesRef.current.get(file) !== upload) return;
            onError(
              file,
              error instanceof Error ? error : new Error('Upload failed'),
            );
          };
          try {
            const ret = await uploadAgentFile([file]);
            if (uploadedFilesRef.current.get(file) !== upload) return;
            if (ret.code === 0) {
              onSuccess(file);
              upload.data = ret.data;
              notifyUploadedFilesChange();
            } else {
              handleError();
            }
          } catch (error) {
            handleError(error);
          }
        });

        // Wait for all uploads to complete
        await Promise.all(uploadPromises);
      } catch (error) {
        // This handles any error that might occur outside the individual upload processes
        console.error('Unexpected error during upload:', error);
      }
    },
    [notifyUploadedFilesChange, uploadAgentFile],
  );

  const onFileReject = React.useCallback((file: File, message: string) => {
    toast(message, {
      description: `"${file.name.length > 20 ? `${file.name.slice(0, 20)}...` : file.name}" has been rejected`,
    });
  }, []);

  /** Reconciles File identities; deleting a record invalidates its pending callbacks. */
  const handleFilesChange = React.useCallback(
    (newFiles: File[]) => {
      let removedFiles = false;
      for (const file of uploadedFilesRef.current.keys()) {
        if (!newFiles.includes(file)) {
          uploadedFilesRef.current.delete(file);
          removedFiles = true;
        }
      }
      for (const file of newFiles) {
        if (!uploadedFilesRef.current.has(file)) {
          uploadedFilesRef.current.set(file, {});
        }
      }
      if (removedFiles) notifyUploadedFilesChange();
      setFiles(newFiles);
    },
    [notifyUploadedFilesChange],
  );

  return (
    <FileUpload
      value={files}
      onValueChange={handleFilesChange}
      onUpload={onUpload}
      onFileReject={onFileReject}
      // TODO： DEFALUT to 5 / 1 for params
      maxFiles={(maxFiles ?? 5) as number}
      className="w-full"
      multiple={!maxFiles || !!(maxFiles && maxFiles > 1)}
    >
      <FileUploadDropzone>
        <div className="flex flex-col items-center gap-1 text-center">
          <div className="flex items-center justify-center rounded-full border p-2.5">
            <Upload className="size-6 text-muted-foreground" />
          </div>
          <p className="font-medium text-sm">Drag & drop files here</p>
          <p className="text-muted-foreground text-xs">
            Or click to browse (max {(maxFiles ?? 5) as number} files)
          </p>
        </div>
        <FileUploadTrigger asChild>
          <Button variant="outline" size="sm" className="mt-2 w-fit">
            Browse files
          </Button>
        </FileUploadTrigger>
      </FileUploadDropzone>
      <FileUploadList>
        {files.map((file, index) => (
          <FileUploadItem key={index} value={file} className="flex-col">
            <div className="flex w-full items-center gap-2">
              <FileUploadItemPreview />
              <FileUploadItemMetadata />
              <FileUploadItemDelete asChild>
                <Button variant="ghost" size="icon" className="size-7">
                  <X />
                </Button>
              </FileUploadItemDelete>
            </div>
            <FileUploadItemProgress />
          </FileUploadItem>
        ))}
      </FileUploadList>
    </FileUpload>
  );
}
