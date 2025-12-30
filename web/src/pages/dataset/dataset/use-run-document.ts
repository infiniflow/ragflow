import { useSetModalState } from '@/hooks/common-hooks';
import { useRunDocument } from '@/hooks/use-document-request';
import { useState } from 'react';

export const useHandleRunDocumentByIds = (id: string) => {
  const { runDocumentByIds, loading } = useRunDocument();
  const [currentId, setCurrentId] = useState<string>('');
  const isLoading = loading && currentId !== '' && currentId === id;
  const { visible, showModal, hideModal } = useSetModalState();
  const handleRunDocumentByIds = async (
    documentId: string,
    isRunning: boolean,
    option?: { delete: boolean; apply_kb: boolean },
  ) => {
    if (isLoading) {
      return;
    }
    setCurrentId(documentId);
    try {
      await runDocumentByIds({
        documentIds: [documentId],
        run: isRunning ? 2 : 1,
        option,
      });
      setCurrentId('');
    } catch (error) {
      setCurrentId('');
    }
    hideModal();
  };

  return {
    handleRunDocumentByIds,
    loading: isLoading,
    visible,
    showModal,
    hideModal,
  };
};
