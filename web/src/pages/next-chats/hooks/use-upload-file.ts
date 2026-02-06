import { FileUploadProps } from '@/components/file-upload';
import {
  useGetChatSearchParams,
  useUploadAndParseFile,
} from '@/hooks/use-chat-request';
import { generateConversationId } from '@/utils/chat';
import { useCallback, useState } from 'react';
import { useChatUrlParams } from './use-chat-url';
import { useSetConversation } from './use-set-conversation';

export function useUploadFile() {
  const { uploadAndParseFile, loading, cancel } = useUploadAndParseFile();
  const [currentFiles, setCurrentFiles] = useState<Record<string, any>[]>([]);
  const [fileMap, setFileMap] = useState<Map<File, Record<string, any>>>(
    new Map(),
  );
  const { setConversation } = useSetConversation();
  const { conversationId, isNew } = useGetChatSearchParams();
  const { setIsNew, setConversationBoth } = useChatUrlParams();

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
        for (const file of files) {
          const ret = await uploadAndParseFile({
            file,
            options,
            conversationId,
          });
          if (ret?.code === 0) {
            const data = ret.data;
            setCurrentFiles((list) => [...list, data]);
            setFileMap((map) => {
              map.set(file, data);
              return map;
            });
          }
        }
      }
    },
    [uploadAndParseFile],
  );

  const createConversationBeforeUploadFile: NonNullable<
    FileUploadProps['onUpload']
  > = useCallback(
    async (files, options) => {
      if (
        (conversationId === '' || isNew === 'true') &&
        Array.isArray(files) &&
        files.length
      ) {
        const currentConversationId = generateConversationId();

        if (conversationId === '') {
          setConversationBoth(currentConversationId, 'true');
        }

        const data = await setConversation(
          files[0].name,
          true,
          conversationId || currentConversationId,
        );
        if (data.code === 0) {
          setIsNew('');
          handleUploadFile(files, options, data.data?.id);
        }
      } else {
        handleUploadFile(files, options);
      }
    },
    [
      conversationId,
      handleUploadFile,
      isNew,
      setConversation,
      setConversationBoth,
      setIsNew,
    ],
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
    handleUploadFile: createConversationBeforeUploadFile,
    files: currentFiles,
    isUploading: loading,
    removeFile,
    clearFiles: clearFiles,
  };
}
