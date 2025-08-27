import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentListByPage } from '@/hooks/use-agent-request';
import { t } from 'i18next';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useCallback } from 'react';
import { AgentCard } from './agent-card';
import { useRenameAgent } from './use-rename-agent';

export default function Agents() {
  const { data, pagination, setPagination, searchString, handleInputChange } =
    useFetchAgentListByPage();
  const { navigateToAgentTemplates } = useNavigatePage();

  const {
    agentRenameLoading,
    initialAgentName,
    onAgentRenameOk,
    agentRenameVisible,
    hideAgentRenameModal,
    showAgentRenameModal,
  } = useRenameAgent();

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  return (
    <section className="flex flex-col w-full flex-1">
      <div className="px-8 pt-8 ">
        <ListFilterBar
          title="Agents"
          searchString={searchString}
          onSearchChange={handleInputChange}
        >
          <Button onClick={navigateToAgentTemplates}>
            <Plus className="mr-2 h-4 w-4" />
            {t('flow.createGraph')}
          </Button>
        </ListFilterBar>
      </div>
      <div className="flex-1 overflow-auto">
        <div className="grid gap-6 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 max-h-[78vh] overflow-auto px-8">
          {data.map((x) => {
            return (
              <AgentCard
                key={x.id}
                data={x}
                showAgentRenameModal={showAgentRenameModal}
              ></AgentCard>
            );
          })}
        </div>
      </div>
      <div className="mt-8 px-8 pb-8">
        <RAGFlowPagination
          {...pick(pagination, 'current', 'pageSize')}
          total={pagination.total}
          onChange={handlePageChange}
        ></RAGFlowPagination>
      </div>
      {agentRenameVisible && (
        <RenameDialog
          hideModal={hideAgentRenameModal}
          onOk={onAgentRenameOk}
          initialName={initialAgentName}
          loading={agentRenameLoading}
        ></RenameDialog>
      )}
    </section>
  );
}
