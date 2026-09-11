import { useDeleteFile } from '@/hooks/use-file-request';
import { useCallback } from 'react';
import { useGetFolderId } from './hooks';

export const useHandleDeleteFile = () => {
  const { deleteFile: removeDocument } = useDeleteFile();
  const parentId = useGetFolderId();

  const handleRemoveFile = useCallback(
    async (fileIds: string[]) => {
      const code = await removeDocument({ fileIds, parentId });

      return code;
    },
    [parentId, removeDocument],
  );

  return { handleRemoveFile };
};
