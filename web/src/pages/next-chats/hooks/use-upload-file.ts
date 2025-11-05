import { FileUploadProps } from '@/components/file-upload';
import { useUploadAndParseFile } from '@/hooks/use-chat-request';
import { useCallback, useState } from 'react';

export function useUploadFile() {
  const { uploadAndParseFile, loading, cancel } = useUploadAndParseFile();
  const [fileIds, setFileIds] = useState<string[]>([]);
  const [fileMap, setFileMap] = useState<Map<File, string>>(new Map());

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
        if (ret?.code === 0 && Array.isArray(ret?.data)) {
          setFileIds((list) => [...list, ...ret.data]);
          setFileMap((map) => {
            map.set(files[0], ret.data[0]);
            return map;
          });
        }
      }
    },
    [uploadAndParseFile],
  );

  const clearFileIds = useCallback(() => {
    setFileIds([]);
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
        setFileIds((list) => list.filter((item) => item !== id));
      }
    },
    [cancel, fileMap, loading],
  );

  return {
    handleUploadFile,
    clearFileIds,
    fileIds,
    isUploading: loading,
    removeFile,
  };
}
