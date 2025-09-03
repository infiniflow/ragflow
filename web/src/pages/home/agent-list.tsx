import { MoreButton } from '@/components/more-button';
import { RenameDialog } from '@/components/rename-dialog';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentListByPage } from '@/hooks/use-agent-request';
import { AgentDropdown } from '../agents/agent-dropdown';
import { useRenameAgent } from '../agents/use-rename-agent';
import { ApplicationCard } from './application-card';

export function Agents() {
  const { data } = useFetchAgentListByPage();
  const { navigateToAgent } = useNavigatePage();
  const {
    agentRenameLoading,
    initialAgentName,
    onAgentRenameOk,
    agentRenameVisible,
    hideAgentRenameModal,
    showAgentRenameModal,
  } = useRenameAgent();

  return (
    <>
      {data.slice(0, 10).map((x) => (
        <ApplicationCard
          key={x.id}
          app={x}
          onClick={navigateToAgent(x.id)}
          moreDropdown={
            <AgentDropdown
              showAgentRenameModal={showAgentRenameModal}
              agent={x}
            >
              <MoreButton></MoreButton>
            </AgentDropdown>
          }
        ></ApplicationCard>
      ))}
      {agentRenameVisible && (
        <RenameDialog
          hideModal={hideAgentRenameModal}
          onOk={onAgentRenameOk}
          initialName={initialAgentName}
          loading={agentRenameLoading}
        ></RenameDialog>
      )}
    </>
  );
}
