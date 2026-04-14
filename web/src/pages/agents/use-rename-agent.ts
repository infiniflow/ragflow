import { useSetModalState } from '@/hooks/common-hooks';
import { useUpdateAgentSetting } from '@/hooks/use-agent-request';
import { IFlow } from '@/interfaces/database/agent';
import { pick } from 'lodash';
import { useCallback, useState } from 'react';

export const useRenameAgent = () => {
  const [agent, setAgent] = useState<IFlow>({} as IFlow);
  const {
    visible: agentRenameVisible,
    hideModal: hideAgentRenameModal,
    showModal: showAgentRenameModal,
  } = useSetModalState();
  const { updateAgentSetting, loading } = useUpdateAgentSetting();

  const onAgentRenameOk = useCallback(
    async (name: string) => {
      const ret = await updateAgentSetting({
        ...pick(agent, ['id', 'avatar', 'description', 'permission']),
        title: name,
      });

      if (ret === 0) {
        hideAgentRenameModal();
      }
    },
    [updateAgentSetting, agent, hideAgentRenameModal],
  );

  const handleShowAgentRenameModal = useCallback(
    async (record: IFlow) => {
      setAgent(record);
      showAgentRenameModal();
    },
    [showAgentRenameModal],
  );

  return {
    agentRenameLoading: loading,
    initialAgentName: agent?.title,
    onAgentRenameOk,
    agentRenameVisible,
    hideAgentRenameModal,
    showAgentRenameModal: handleShowAgentRenameModal,
  };
};
