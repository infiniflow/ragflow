import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { RenameDialog } from '@/components/rename-dialog';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentListByPage } from '@/hooks/use-agent-request';
import { AgentListItem, AgentListItemType } from '@/interfaces/database/agent';
import { useEffect, useMemo } from 'react';
import { AgentDropdown } from '../agents/agent-dropdown';
import { useRenameAgent } from '../agents/use-rename-agent';

export function Agents({
  setListLength,
  setLoading,
}: {
  setListLength: (length: number) => void;
  setLoading?: (loading: boolean) => void;
}) {
  const { data, loading } = useFetchAgentListByPage();
  const { navigateToAgent } = useNavigatePage();
  const {
    agentRenameLoading,
    initialAgentName,
    onAgentRenameOk,
    agentRenameVisible,
    hideAgentRenameModal,
    showAgentRenameModal,
  } = useRenameAgent();

  const agentList = useMemo(
    () =>
      data.filter(
        (item): item is AgentListItem & { type: AgentListItemType.Agent } =>
          item.type !== AgentListItemType.CompilationTemplateGroup,
      ),
    [data],
  );

  useEffect(() => {
    setListLength(agentList?.length || 0);
    setLoading?.(loading || false);
  }, [agentList, setListLength, loading, setLoading]);

  return (
    <>
      {agentList.slice(0, 10).map((x) => (
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
