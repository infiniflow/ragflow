import { useSetModalState } from '@/hooks/common-hooks';
import { EmptyDsl, useSetAgent } from '@/hooks/use-agent-request';
import { DSL } from '@/interfaces/database/agent';
import { AgentCategory } from '@/pages/agent/constant';
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

  const handleCreateAgentOrPipeline = useCallback(
    async (data: FormSchemaType) => {
      const ret = await setAgent({
        title: data.name,
        dsl: EmptyDsl as DSL,
        canvas_category:
          data.type === FlowType.Agent
            ? AgentCategory.AgentCanvas
            : AgentCategory.DataflowCanvas,
      });

      if (ret.code === 0) {
        hideCreatingModal();
      }
    },
    [hideCreatingModal, setAgent],
  );

  return {
    loading: loading,
    creatingVisible,
    hideCreatingModal,
    showCreatingModal,
    handleCreateAgentOrPipeline,
  };
}
