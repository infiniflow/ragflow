import { useSetModalState } from '@/hooks/common-hooks';
import { useSetAgent } from '@/hooks/use-agent-request';
import { EmptyDsl, useSetDataflow } from '@/hooks/use-dataflow-request';
import { useCallback } from 'react';
import { FlowType } from '../constant';
import { FormSchemaType } from '../create-agent-form';

export function useCreateAgentOrPipeline() {
  const { loading, setAgent } = useSetAgent();
  const { loading: dataflowLoading, setDataflow } = useSetDataflow();
  const {
    visible: creatingVisible,
    hideModal: hideCreatingModal,
    showModal: showCreatingModal,
  } = useSetModalState();

  const createAgent = useCallback(
    async (name: string) => {
      return setAgent({ title: name, dsl: EmptyDsl });
    },
    [setAgent],
  );

  const handleCreateAgentOrPipeline = useCallback(
    async (data: FormSchemaType) => {
      if (data.type === FlowType.Agent) {
        const ret = await createAgent(data.name);
        if (ret.code === 0) {
          hideCreatingModal();
        }
      } else {
        setDataflow({
          title: data.name,
          dsl: EmptyDsl,
        });
      }
    },
    [createAgent, hideCreatingModal, setDataflow],
  );

  return {
    loading: loading || dataflowLoading,
    creatingVisible,
    hideCreatingModal,
    showCreatingModal,
    handleCreateAgentOrPipeline,
  };
}
