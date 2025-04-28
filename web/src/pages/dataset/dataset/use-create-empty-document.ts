import { useSetModalState } from '@/hooks/common-hooks';
import { useCreateDocument } from '@/hooks/use-document-request';
import { useCallback } from 'react';

export const useCreateEmptyDocument = () => {
  const { createDocument, loading } = useCreateDocument();

  const {
    visible: createVisible,
    hideModal: hideCreateModal,
    showModal: showCreateModal,
  } = useSetModalState();

  const onCreateOk = useCallback(
    async (name: string) => {
      const ret = await createDocument(name);
      if (ret === 0) {
        hideCreateModal();
      }
    },
    [hideCreateModal, createDocument],
  );

  return {
    createLoading: loading,
    onCreateOk,
    createVisible,
    hideCreateModal,
    showCreateModal,
  };
};
