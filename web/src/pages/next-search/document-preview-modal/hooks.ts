import { useSetModalState } from '@/hooks/common-hooks';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { useCallback, useState } from 'react';

export const useClickDrawer = () => {
  const { visible, showModal, hideModal } = useSetModalState();
  const [selectedChunk, setSelectedChunk] = useState<IReferenceChunk>(
    {} as IReferenceChunk,
  );
  const [documentId, setDocumentId] = useState<string>('');

  const clickDocumentButton = useCallback(
    (documentId: string, chunk: IReferenceChunk) => {
      showModal();
      setSelectedChunk(chunk);
      setDocumentId(documentId);
    },
    [showModal],
  );

  return {
    clickDocumentButton,
    visible,
    showModal,
    hideModal,
    selectedChunk,
    documentId,
  };
};
