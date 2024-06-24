import { useSetModalState } from '@/hooks/commonHooks';
import { useFetchFlowList, useSetFlow } from '@/hooks/flow-hooks';
import { useCallback, useState } from 'react';
import { useNavigate } from 'umi';
import { dsl } from '../mock';

export const useFetchDataOnMount = () => {
  const { data, loading } = useFetchFlowList();

  return { list: data, loading };
};

export const useSaveFlow = () => {
  const [currentFlow, setCurrentFlow] = useState({});
  const {
    visible: flowSettingVisible,
    hideModal: hideFlowSettingModal,
    showModal: showFileRenameModal,
  } = useSetModalState();
  const { loading, setFlow } = useSetFlow();
  const navigate = useNavigate();

  const onFlowOk = useCallback(
    async (title: string) => {
      const ret = await setFlow({ title, dsl });

      if (ret?.retcode === 0) {
        hideFlowSettingModal();
        navigate(`/flow/${ret.data.id}`);
      }
    },
    [setFlow, hideFlowSettingModal, navigate],
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
