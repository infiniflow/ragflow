import { useSetModalState } from '@/hooks/common-hooks';
import { useMoveFile } from '@/hooks/use-file-request';
import { useCallback, useState } from 'react';

export const useHandleMoveFile = () => {
  const {
    visible: moveFileVisible,
    hideModal: hideMoveFileModal,
    showModal: showMoveFileModal,
  } = useSetModalState();
  const { moveFile, loading } = useMoveFile();
  const [sourceFileIds, setSourceFileIds] = useState<string[]>([]);

  const onMoveFileOk = useCallback(
    async (targetFolderId: string) => {
      const ret = await moveFile({
        src_file_ids: sourceFileIds,
        dest_file_id: targetFolderId,
      });

      if (ret === 0) {
        // setSelectedRowKeys([]);
        hideMoveFileModal();
      }
      return ret;
    },
    [moveFile, hideMoveFileModal, sourceFileIds],
  );

  const handleShowMoveFileModal = useCallback(
    (ids: string[]) => {
      setSourceFileIds(ids);
      showMoveFileModal();
    },
    [showMoveFileModal],
  );

  return {
    initialValue: '',
    moveFileLoading: loading,
    onMoveFileOk,
    moveFileVisible,
    hideMoveFileModal,
    showMoveFileModal: handleShowMoveFileModal,
  };
};

export type UseMoveDocumentReturnType = ReturnType<typeof useHandleMoveFile>;

export type UseMoveDocumentShowType = Pick<
  ReturnType<typeof useHandleMoveFile>,
  'showMoveFileModal'
>;
