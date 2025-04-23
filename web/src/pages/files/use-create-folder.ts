import { useSetModalState } from '@/hooks/common-hooks';
import { useCreateFolder } from '@/hooks/use-file-request';
import { useCallback } from 'react';
import { useGetFolderId } from './hooks';

export const useHandleCreateFolder = () => {
  const {
    visible: folderCreateModalVisible,
    hideModal: hideFolderCreateModal,
    showModal: showFolderCreateModal,
  } = useSetModalState();
  const { createFolder, loading } = useCreateFolder();
  const id = useGetFolderId();

  const onFolderCreateOk = useCallback(
    async (name: string) => {
      const ret = await createFolder({ parentId: id, name });

      if (ret === 0) {
        hideFolderCreateModal();
      }
    },
    [createFolder, hideFolderCreateModal, id],
  );

  return {
    folderCreateLoading: loading,
    onFolderCreateOk,
    folderCreateModalVisible,
    hideFolderCreateModal,
    showFolderCreateModal,
  };
};
