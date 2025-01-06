import { useSetModalState } from '@/hooks/common-hooks';
import {
  useFetchFlowList,
  useFetchFlowTemplates,
  useSetFlow,
} from '@/hooks/flow-hooks';
import { useCallback } from 'react';
import { useNavigate } from 'umi';

export const useFetchDataOnMount = () => {
  const { data, loading } = useFetchFlowList();

  return { list: data, loading };
};

export const useSaveFlow = () => {
  const {
    visible: flowSettingVisible,
    hideModal: hideFlowSettingModal,
    showModal: showFileRenameModal,
  } = useSetModalState();
  const { loading, setFlow } = useSetFlow();
  const navigate = useNavigate();
  const { data: list } = useFetchFlowTemplates();

  const onFlowOk = useCallback(
    async (title: string, templateId: string) => {
      const templateItem = list.find((x) => x.id === templateId);

      let dsl = templateItem?.dsl;
      const ret = await setFlow({
        title,
        dsl,
        avatar: templateItem?.avatar,
      });

      if (ret?.code === 0) {
        hideFlowSettingModal();
        navigate(`/flow/${ret.data.id}`);
      }
    },
    [setFlow, hideFlowSettingModal, navigate, list],
  );

  return {
    flowSettingLoading: loading,
    initialFlowName: '',
    onFlowOk,
    flowSettingVisible,
    hideFlowSettingModal,
    showFlowSettingModal: showFileRenameModal,
  };
};
