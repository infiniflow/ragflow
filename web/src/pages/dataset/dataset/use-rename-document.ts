import { useSetModalState } from '@/hooks/common-hooks';
import { useSaveDocumentName } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useCallback, useState } from 'react';

export const useRenameDocument = () => {
  const { saveName, loading } = useSaveDocumentName();
  const [record, setRecord] = useState<IDocumentInfo>();

  const {
    visible: renameVisible,
    hideModal: hideRenameModal,
    showModal: showRenameModal,
  } = useSetModalState();

  const onRenameOk = useCallback(
    async (name: string) => {
      if (record?.id) {
        const ret = await saveName({ documentId: record.id, name });
        if (ret === 0) {
          hideRenameModal();
        }
      }
    },
    [record?.id, saveName, hideRenameModal],
  );

  const handleShow = useCallback(
    (row: IDocumentInfo) => {
      setRecord(row);
      showRenameModal();
    },
    [showRenameModal],
  );

  return {
    renameLoading: loading,
    onRenameOk,
    renameVisible,
    hideRenameModal,
    showRenameModal: handleShow,
    initialName: record?.name,
  };
};

export type UseRenameDocumentShowType = Pick<
  ReturnType<typeof useRenameDocument>,
  'showRenameModal'
>;
