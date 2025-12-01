import { FileUploadProps } from '@/components/file-upload';
import { useUploadAndParseFile } from '@/hooks/use-chat-request';
import { useCallback, useState } from 'react';

export function useUploadFile() {
  const { uploadAndParseFile, loading, cancel } = useUploadAndParseFile();
  const [currentFiles, setCurrentFiles] = useState<Record<string, any>[]>([]);
  const [fileMap, setFileMap] = useState<Map<File, Record<string, any>>>(
    new Map(),
  );

  type FileUploadParameters = Parameters<
    NonNullable<FileUploadProps['onUpload']>
  >;

  const handleUploadFile = useCallback(
    async (
      files: FileUploadParameters[0],
      options: FileUploadParameters[1],
      conversationId?: string,
    ) => {
      if (Array.isArray(files) && files.length) {
        const file = files[0];
        const ret = await uploadAndParseFile({ file, options, conversationId });
        if (ret?.code === 0) {
          const data = ret.data;
          setCurrentFiles((list) => [...list, data]);
          setFileMap((map) => {
            map.set(files[0], data);
            return map;
          });
        }
      }
    },
    [uploadAndParseFile],
  );

  const clearFiles = useCallback(() => {
    setCurrentFiles([]);
    setFileMap(new Map());
  }, []);

  const removeFile = useCallback(
    (file: File) => {
      if (loading) {
        cancel();
        return;
      }
      const id = fileMap.get(file);
      if (id) {
        setCurrentFiles((list) => list.filter((item) => item !== id));
      }
    },
    [cancel, fileMap, loading],
  );

  return {
    handleUploadFile,
    files: currentFiles,
    isUploading: loading,
    removeFile,
    clearFiles: clearFiles,
  };
}
