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
import { Upload, X } from 'lucide-react';
import * as React from 'react';
import { toast } from 'sonner';

export function Uploader() {
  const [isUploading, setIsUploading] = React.useState(false);
  const [files, setFiles] = React.useState<File[]>([]);

  const onUpload: NonNullable<FileUploadProps['onUpload']> = React.useCallback(
    async (files, { onProgress }) => {
      try {
        setIsUploading(true);
        // const res = await uploadFiles('imageUploader', {
        //   files,
        //   onUploadProgress: ({ file, progress }) => {
        //     onProgress(file, progress);
        //   },
        // });
      } catch (error) {
        setIsUploading(false);

        // if (error instanceof UploadThingError) {
        //   const errorMessage =
        //     error.data && 'error' in error.data
        //       ? error.data.error
        //       : 'Upload failed';
        //   toast.error(errorMessage);
        //   return;
        // }

        toast.error(
          error instanceof Error ? error.message : 'An unknown error occurred',
        );
      } finally {
        setIsUploading(false);
      }
    },
    [],
  );

  const onFileReject = React.useCallback((file: File, message: string) => {
    toast(message, {
      description: `"${file.name.length > 20 ? `${file.name.slice(0, 20)}...` : file.name}" has been rejected`,
    });
  }, []);

  return (
    <FileUpload
      // accept="image/*"
      // maxFiles={2}
      // maxSize={4 * 1024 * 1024}
      className="w-full"
      onAccept={(files) => setFiles(files)}
      onUpload={onUpload}
      onFileReject={onFileReject}
      multiple
      disabled={isUploading}
    >
      <FileUploadDropzone>
        <div className="flex flex-col items-center gap-1 text-center">
          <div className="flex items-center justify-center rounded-full border p-2.5">
            <Upload className="size-6 text-muted-foreground" />
          </div>
          <p className="font-medium text-sm">Drag & drop images here</p>
          <p className="text-muted-foreground text-xs">
            Or click to browse (max 2 files, up to 4MB each)
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
          <FileUploadItem key={index} value={file}>
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
