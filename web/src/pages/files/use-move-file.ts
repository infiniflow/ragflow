import { useSetModalState } from '@/hooks/common-hooks';
import { UseRowSelectionType } from '@/hooks/logic-hooks/use-row-selection';
import { useMoveFile } from '@/hooks/use-file-request';
import { useCallback, useRef, useState } from 'react';

export const useHandleMoveFile = ({
  clearRowSelection,
}: Pick<UseRowSelectionType, 'clearRowSelection'>) => {
  const {
    visible: moveFileVisible,
    hideModal: hideMoveFileModal,
    showModal: showMoveFileModal,
  } = useSetModalState();
  const { moveFile, loading } = useMoveFile();
  const [sourceFileIds, setSourceFileIds] = useState<string[]>([]);
  const isBulkRef = useRef(false);

  const onMoveFileOk = useCallback(
    async (targetFolderId: string) => {
      const ret = await moveFile({
        src_file_ids: sourceFileIds,
        dest_file_id: targetFolderId,
      });

      if (ret === 0) {
        if (isBulkRef.current) {
          clearRowSelection();
        }
        hideMoveFileModal();
      }
      return ret;
    },
    [moveFile, sourceFileIds, hideMoveFileModal, clearRowSelection],
  );

  const handleShowMoveFileModal = useCallback(
    (ids: string[], isBulk = false) => {
      isBulkRef.current = isBulk;
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
