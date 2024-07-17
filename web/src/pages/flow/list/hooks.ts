import { useSetModalState } from '@/hooks/common-hooks';
import {
  useFetchFlowList,
  useFetchFlowTemplates,
  useSetFlow,
} from '@/hooks/flow-hooks';
import { useCallback, useState } from 'react';
import { useNavigate } from 'umi';
// import { dsl } from '../mock';
// import headhunterZhComponents from '../../../../../graph/test/dsl_examples/headhunter_zh.json';
// import dslJson from '../../../../../dls.json';
// import customerServiceBase from '../../../../../graph/test/dsl_examples/customer_service.json';
// import customerService from '../customer_service.json';
// import interpreterBase from '../../../../../graph/test/dsl_examples/interpreter.json';
// import interpreter from '../interpreter.json';

// import retrievalRelevantRewriteAndGenerateBase from '../../../../../graph/test/dsl_examples/retrieval_relevant_rewrite_and_generate.json';
// import retrievalRelevantRewriteAndGenerate from '../retrieval_relevant_rewrite_and_generate.json';

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
  const { data: list } = useFetchFlowTemplates();

  const onFlowOk = useCallback(
    async (title: string, templateId: string) => {
      const templateItem = list.find((x) => x.id === templateId);

      let dsl = templateItem?.dsl;
      // if (dsl) {
      //   dsl.graph = headhunter_zh;
      // }
      const ret = await setFlow({
        title,
        dsl,
        avatar: templateItem?.avatar,
        // dsl: dslJson,
        // dsl: {
        //   ...retrievalRelevantRewriteAndGenerateBase,
        //   graph: retrievalRelevantRewriteAndGenerate,
        // },
      });

      if (ret?.retcode === 0) {
        hideFlowSettingModal();
        navigate(`/flow/${ret.data.id}`);
      }
    },
    [setFlow, hideFlowSettingModal, navigate, list],
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
