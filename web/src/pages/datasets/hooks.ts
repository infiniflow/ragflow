import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useCreateKnowledge } from '@/hooks/use-knowledge-request';
import { useCallback, useState } from 'react';

export const useSearchKnowledge = () => {
  const [searchString, setSearchString] = useState<string>('');

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSearchString(e.target.value);
  };
  return {
    searchString,
    handleInputChange,
  };
};

export const useSaveKnowledge = () => {
  const { visible: visible, hideModal, showModal } = useSetModalState();
  const { loading, createKnowledge } = useCreateKnowledge();
  const { navigateToDataset } = useNavigatePage();

  const onCreateOk = useCallback(
    async (name: string) => {
      const ret = await createKnowledge({
        name,
      });

      if (ret?.code === 0) {
        hideModal();
        navigateToDataset(ret.data.kb_id)();
      }
    },
    [createKnowledge, hideModal, navigateToDataset],
  );

  return {
    loading,
    onCreateOk,
    visible,
    hideModal,
    showModal,
  };
};
