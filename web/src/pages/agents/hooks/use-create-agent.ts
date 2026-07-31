import { AgentCategory } from '@/constants/agent';
import { useSetModalState } from '@/hooks/common-hooks';
import { useSetAgent } from '@/hooks/use-agent-request';

import { initialEmptyDsl } from '@/pages/agent/utils/dsl-bridge';
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
      const isAgent = data.type === FlowType.Agent;
      const ret = await setAgent({
        title: data.name,
        dsl: initialEmptyDsl(isAgent),
        canvas_category: isAgent
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
