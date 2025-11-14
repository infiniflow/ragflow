import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { RenameDialog } from '@/components/rename-dialog';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentListByPage } from '@/hooks/use-agent-request';
import { AgentDropdown } from '../agents/agent-dropdown';
import { useRenameAgent } from '../agents/use-rename-agent';

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
        <HomeCard
          key={x.id}
          data={{ name: x.title, ...x } as any}
          onClick={navigateToAgent(x.id)}
          moreDropdown={
            <AgentDropdown
              showAgentRenameModal={showAgentRenameModal}
              agent={x}
            >
              <MoreButton></MoreButton>
            </AgentDropdown>
          }
        ></HomeCard>
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
