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
import { useUploadCanvasFile } from '@/hooks/use-agent-request';
import { Upload, X } from 'lucide-react';
import * as React from 'react';
import { toast } from 'sonner';

type FileUploadDirectUploadProps = {
  value: Record<string, any>;
  onChange(value: Record<string, any>): void;
};

export function FileUploadDirectUpload({
  onChange,
}: FileUploadDirectUploadProps) {
  const [files, setFiles] = React.useState<File[]>([]);

  const { uploadCanvasFile } = useUploadCanvasFile();

  const onUpload: NonNullable<FileUploadProps['onUpload']> = React.useCallback(
    async (files, { onSuccess, onError }) => {
      try {
        const uploadPromises = files.map(async (file) => {
          const handleError = (error?: any) => {
            onError(
              file,
              error instanceof Error ? error : new Error('Upload failed'),
            );
          };
          try {
            const ret = await uploadCanvasFile([file]);
            if (ret.code === 0) {
              onSuccess(file);
              onChange(ret.data);
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
    [onChange, uploadCanvasFile],
  );

  const onFileReject = React.useCallback((file: File, message: string) => {
    toast(message, {
      description: `"${file.name.length > 20 ? `${file.name.slice(0, 20)}...` : file.name}" has been rejected`,
    });
  }, []);

  return (
    <FileUpload
      value={files}
      onValueChange={setFiles}
      onUpload={onUpload}
      onFileReject={onFileReject}
      maxFiles={1}
      className="w-full"
      multiple={false}
    >
      <FileUploadDropzone>
        <div className="flex flex-col items-center gap-1 text-center">
          <div className="flex items-center justify-center rounded-full border p-2.5">
            <Upload className="size-6 text-muted-foreground" />
          </div>
          <p className="font-medium text-sm">Drag & drop files here</p>
          <p className="text-muted-foreground text-xs">
            Or click to browse (max 1 files)
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
