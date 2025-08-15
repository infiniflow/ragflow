import { FileUploadProps } from '@/components/file-upload';
import { useUploadAndParseFile } from '@/hooks/use-chat-request';
import { useCallback, useState } from 'react';

export function useUploadFile() {
  const { uploadAndParseFile, loading } = useUploadAndParseFile();
  const [fileIds, setFileIds] = useState<string[]>([]);

  const handleUploadFile: NonNullable<FileUploadProps['onUpload']> =
    useCallback(
      async (files) => {
        if (Array.isArray(files) && files.length) {
          const ret = await uploadAndParseFile(files[0]);
          if (ret.code === 0 && Array.isArray(ret.data)) {
            setFileIds((list) => [...list, ...ret.data]);
          }
        }
      },
      [uploadAndParseFile],
    );

  const clearFileIds = useCallback(() => {
    setFileIds([]);
  }, []);

  return { handleUploadFile, clearFileIds, fileIds, isUploading: loading };
}
