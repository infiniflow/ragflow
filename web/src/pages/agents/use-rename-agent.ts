import { useSetModalState } from '@/hooks/common-hooks';
import { useUpdateKnowledge } from '@/hooks/use-knowledge-request';
import { IFlow } from '@/interfaces/database/flow';
import { omit } from 'lodash';
import { useCallback, useState } from 'react';

export const useRenameAgent = () => {
  const [dataset, setDataset] = useState<IFlow>({} as IFlow);
  const {
    visible: datasetRenameVisible,
    hideModal: hideDatasetRenameModal,
    showModal: showDatasetRenameModal,
  } = useSetModalState();
  const { saveKnowledgeConfiguration, loading } = useUpdateKnowledge(true);

  const onDatasetRenameOk = useCallback(
    async (name: string) => {
      const ret = await saveKnowledgeConfiguration({
        ...omit(dataset, [
          'id',
          'update_time',
          'nickname',
          'tenant_avatar',
          'tenant_id',
        ]),
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
    async (record: IFlow) => {
      setDataset(record);
      showDatasetRenameModal();
    },
    [showDatasetRenameModal],
  );

  return {
    datasetRenameLoading: loading,
    initialDatasetName: dataset?.title,
    onDatasetRenameOk,
    datasetRenameVisible,
    hideDatasetRenameModal,
    showDatasetRenameModal: handleShowDatasetRenameModal,
  };
};
