import { useRunDocument } from '@/hooks/use-document-request';
import { useState } from 'react';

export const useHandleRunDocumentByIds = (id: string) => {
  const { runDocumentByIds, loading } = useRunDocument();
  const [currentId, setCurrentId] = useState<string>('');
  const isLoading = loading && currentId !== '' && currentId === id;

  const handleRunDocumentByIds = async (
    documentId: string,
    isRunning: boolean,
    shouldDelete: boolean = false,
  ) => {
    if (isLoading) {
      return;
    }
    setCurrentId(documentId);
    try {
      await runDocumentByIds({
        documentIds: [documentId],
        run: isRunning ? 2 : 1,
        shouldDelete,
      });
      setCurrentId('');
    } catch (error) {
      setCurrentId('');
    }
  };

  return {
    handleRunDocumentByIds,
    loading: isLoading,
  };
};
