import { useSetModalState } from '@/hooks/commonHooks';
import { useFetchFlowList, useSetFlow } from '@/hooks/flow-hooks';
import { useCallback, useState } from 'react';
import { dsl } from '../mock';

export const useFetchDataOnMount = () => {
  const { data, loading } = useFetchFlowList();
  console.info(data, loading);
};

export const useSaveFlow = () => {
  const [currentFlow, setCurrentFlow] = useState({});
  const {
    visible: flowSettingVisible,
    hideModal: hideFlowSettingModal,
    showModal: showFileRenameModal,
  } = useSetModalState();
  const { loading, setFlow } = useSetFlow();

  const onFlowOk = useCallback(
    async (title: string) => {
      const ret = await setFlow({ title, dsl });

      if (ret === 0) {
        hideFlowSettingModal();
      }
    },
    [setFlow, hideFlowSettingModal],
  );

  const handleShowFlowSettingModal = useCallback(
    async (record: any) => {
      setCurrentFlow(record);
      showFileRenameModal();
    },
    [showFileRenameModal],
  );

  return {
    flowSettingLoading: loading,
    initialFlowName: '',
    onFlowOk,
    flowSettingVisible,
    hideFlowSettingModal,
    showFlowSettingModal: handleShowFlowSettingModal,
  };
};
