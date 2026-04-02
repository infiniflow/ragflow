import { useSetModalState } from '@/hooks/common-hooks';
import { useUpdateKnowledge } from '@/hooks/use-knowledge-request';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { useCallback, useState } from 'react';

export const useRenameDataset = () => {
  const [dataset, setDataset] = useState<IKnowledge>({} as IKnowledge);
  const {
    visible: datasetRenameVisible,
    hideModal: hideDatasetRenameModal,
    showModal: showDatasetRenameModal,
  } = useSetModalState();
  const { saveKnowledgeConfiguration, loading } = useUpdateKnowledge(true);

  const onDatasetRenameOk = useCallback(
    async (name: string) => {
      const ret = await saveKnowledgeConfiguration({
        kb_id: dataset.id,
        name,
      });

      if (ret.code === 0) {
        hideDatasetRenameModal();
      }
    },
    [saveKnowledgeConfiguration, dataset, hideDatasetRenameModal],
  );

  const handleShowDatasetRenameModal = useCallback(
    async (record: IKnowledge) => {
      setDataset(record);
      showDatasetRenameModal();
    },
    [showDatasetRenameModal],
  );

  return {
    datasetRenameLoading: loading,
    initialDatasetName: dataset?.name,
    onDatasetRenameOk,
    datasetRenameVisible,
    hideDatasetRenameModal,
    showDatasetRenameModal: handleShowDatasetRenameModal,
  };
};
