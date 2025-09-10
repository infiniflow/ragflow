import { useSetModalState } from '@/hooks/common-hooks';
import { EmptyDsl, useSetAgent } from '@/hooks/use-agent-request';
import { DSL } from '@/interfaces/database/agent';
import { useCallback } from 'react';
import { FlowType } from '../constant';
import { FormSchemaType } from '../create-agent-form';

export function useCreateAgentOrPipeline() {
  const { loading, setAgent } = useSetAgent();
  const {
    visible: creatingVisible,
    hideModal: hideCreatingModal,
    showModal: showCreatingModal,
  } = useSetModalState();

  const createAgent = useCallback(
    async (name: string) => {
      return setAgent({ title: name, dsl: EmptyDsl as DSL });
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
      }
    },
    [createAgent, hideCreatingModal],
  );

  return {
    loading,
    creatingVisible,
    hideCreatingModal,
    showCreatingModal,
    handleCreateAgentOrPipeline,
  };
}
